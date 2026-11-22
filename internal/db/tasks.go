package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Background work lives in Postgres (tables task and task_event) rather than in server
// memory, so a task survives a restart, any instance can run it, and any instance can
// answer progress polls for it.

// Task is a unit of background work claimed by a worker.
type Task struct {
	ID          string
	UserID      string
	Kind        string
	Subject     string // the document or job id
	Attempts    int    // including the current one
	MaxAttempts int
	Input       []byte
}

// NewTask describes work to enqueue.
type NewTask struct {
	UserID, Kind, Subject string
	Input                 []byte
	MaxAttempts           int // 0 means 3
}

// Enqueue adds a task and its first progress event.
func (d *DB) Enqueue(ctx context.Context, t NewTask, first map[string]any) (string, error) {
	if t.MaxAttempts <= 0 {
		t.MaxAttempts = 3
	}
	s, err := d.sealerFor(ctx, t.UserID)
	if err != nil {
		return "", err
	}
	id := newUUID()
	input, err := s.bytes("task.input", id, t.Input)
	if err != nil {
		return "", err
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO task (id,user_id,kind,subject,input_enc,max_attempts) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, t.UserID, t.Kind, t.Subject, input, t.MaxAttempts); err != nil {
		return "", err
	}
	if first != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO task_event (task_id,data) VALUES ($1,$2)`, id, first); err != nil {
			return "", err
		}
	}
	return id, tx.Commit(ctx)
}

// Claim takes the oldest runnable task of the given kinds: a queued one whose time has
// come, or a running one whose lease ran out (its server stopped). It counts an attempt
// and leases the task until now+lease. It returns nil when there is nothing to do.
func (d *DB) Claim(ctx context.Context, kinds []string, lease time.Duration) (*Task, error) {
	var t Task
	var enc []byte
	err := d.Pool.QueryRow(ctx, `
		UPDATE task SET status='running', attempts=attempts+1, lease_until=now()+make_interval(secs => $2), updated_at=now()
		WHERE id = (
			SELECT id FROM task
			WHERE kind = ANY($1) AND (
				(status='queued' AND run_after <= now()) OR
				(status='running' AND lease_until < now()))
			ORDER BY run_after
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING id, user_id, kind, subject::text, attempts, max_attempts, input_enc, input`,
		kinds, lease.Seconds()).Scan(&t.ID, &t.UserID, &t.Kind, &t.Subject, &t.Attempts, &t.MaxAttempts, &enc, &t.Input)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if enc != nil {
		// A task whose input can't be decrypted (its owner was deleted meanwhile) is still
		// returned, with no input, so the worker finishes it instead of retrying forever.
		s, err := d.sealerFor(ctx, t.UserID)
		if err == nil {
			t.Input, err = s.openBytes("task.input", t.ID, enc)
		}
		if err != nil {
			t.Input = nil
		}
	}
	return &t, nil
}

// Renew extends a running task's lease. Workers call it while they work.
func (d *DB) Renew(ctx context.Context, id string, lease time.Duration) error {
	_, err := d.Pool.Exec(ctx, `UPDATE task SET lease_until=now()+make_interval(secs => $2) WHERE id=$1 AND status='running'`, id, lease.Seconds())
	return err
}

// Emit records a progress event for a task.
func (d *DB) Emit(ctx context.Context, taskID string, ev map[string]any) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO task_event (task_id,data) VALUES ($1,$2)`, taskID, ev)
	return err
}

// FinishTask marks a task done or failed for good and clears its input.
func (d *DB) FinishTask(ctx context.Context, id string, failed bool, msg string) error {
	status := "done"
	if failed {
		status = "failed"
	}
	_, err := d.Pool.Exec(ctx, `UPDATE task SET status=$2, last_error=NULLIF($3,''), input=NULL, input_enc=NULL, lease_until=NULL, updated_at=now() WHERE id=$1`, id, status, msg)
	return err
}

// RetryTask puts a task back in the queue to run again after delay.
func (d *DB) RetryTask(ctx context.Context, id string, delay time.Duration, msg string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE task SET status='queued', run_after=now()+make_interval(secs => $2), last_error=$3, lease_until=NULL, updated_at=now() WHERE id=$1`,
		id, delay.Seconds(), msg)
	return err
}

// ReleaseTask hands a task back without counting the attempt, for a clean shutdown.
func (d *DB) ReleaseTask(ctx context.Context, id string) error {
	_, err := d.Pool.Exec(ctx, `UPDATE task SET status='queued', attempts=greatest(attempts-1,0), lease_until=NULL, updated_at=now() WHERE id=$1 AND status='running'`, id)
	return err
}

// TaskProgress is what a progress poll returns.
type TaskProgress struct {
	Found  bool
	Events []json.RawMessage
	Next   int64
	Done   bool
}

// Progress returns the events after `after` of the latest task the user started on subject.
func (d *DB) Progress(ctx context.Context, userID, subject string, after int64) (TaskProgress, error) {
	out := TaskProgress{Next: after, Events: []json.RawMessage{}}
	var taskID, status string
	err := d.Pool.QueryRow(ctx, `SELECT id, status FROM task WHERE user_id=$1 AND subject=$2 ORDER BY created_at DESC LIMIT 1`,
		userID, subject).Scan(&taskID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Found = true
	rows, err := d.Pool.Query(ctx, `SELECT id, data FROM task_event WHERE task_id=$1 AND id > $2 ORDER BY id LIMIT 200`, taskID, after)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var data json.RawMessage
		if err := rows.Scan(&id, &data); err != nil {
			return out, err
		}
		out.Events = append(out.Events, data)
		out.Next = id
	}
	// Done only once every event has been read.
	out.Done = (status == "done" || status == "failed") && len(out.Events) < 200
	return out, rows.Err()
}

// PurgeTasks deletes finished tasks (and their events) an hour after they finish.
func (d *DB) PurgeTasks(ctx context.Context) {
	_, _ = d.Pool.Exec(ctx, `DELETE FROM task WHERE status IN ('done','failed') AND updated_at < now() - interval '1 hour'`)
}
