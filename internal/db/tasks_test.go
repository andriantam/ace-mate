package db_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/andriantam/ace-mate/internal/db"
)

// One database for these tests: starting an embedded Postgres takes a few seconds.
func TestTasksAndConsent(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	alice, _ := d.UpsertOAuthUser(ctx, "google", "sub-alice", "alice@example.com", "Alice", "")
	bob, _ := d.UpsertOAuthUser(ctx, "google", "sub-bob", "bob@example.com", "Bob", "")
	const subject = "00000000-0000-4000-8000-000000000001"
	kinds := []string{"document"}

	t.Run("claim, progress and finish", func(t *testing.T) {
		id, err := d.Enqueue(ctx, db.NewTask{UserID: alice, Kind: "document", Subject: subject, Input: []byte("file")}, map[string]any{"type": "progress", "step": "received"})
		if err != nil {
			t.Fatal(err)
		}
		task, err := d.Claim(ctx, kinds, time.Minute)
		if err != nil || task == nil || task.ID != id || task.Attempts != 1 || string(task.Input) != "file" {
			t.Fatalf("claim: %+v %v", task, err)
		}
		if again, _ := d.Claim(ctx, kinds, time.Minute); again != nil {
			t.Fatal("a leased task must not be claimed twice")
		}
		_ = d.Emit(ctx, id, map[string]any{"type": "done"})

		p, err := d.Progress(ctx, alice, subject, 0)
		if err != nil || !p.Found || len(p.Events) != 2 || p.Done {
			t.Fatalf("progress while running: %+v %v", p, err)
		}
		var last map[string]any
		_ = json.Unmarshal(p.Events[1], &last)
		if last["type"] != "done" {
			t.Fatalf("events out of order: %s", p.Events[1])
		}
		if rest, _ := d.Progress(ctx, alice, subject, p.Next); len(rest.Events) != 0 {
			t.Fatal("polling after the last event must return nothing new")
		}
		if other, _ := d.Progress(ctx, bob, subject, 0); other.Found {
			t.Fatal("another user must not see this task's progress")
		}

		if err := d.FinishTask(ctx, id, false, ""); err != nil {
			t.Fatal(err)
		}
		if p, _ := d.Progress(ctx, alice, subject, p.Next); !p.Done {
			t.Fatal("finished task should report done")
		}
		var input []byte
		_ = d.Pool.QueryRow(ctx, `SELECT input FROM task WHERE id=$1`, id).Scan(&input)
		if input != nil {
			t.Fatal("the uploaded file must be cleared when the task finishes")
		}
	})

	t.Run("an expired lease is picked up again", func(t *testing.T) {
		id, _ := d.Enqueue(ctx, db.NewTask{UserID: alice, Kind: "document", Subject: subject}, nil)
		first, _ := d.Claim(ctx, kinds, time.Minute)
		if first == nil || first.ID != id {
			t.Fatalf("claim: %+v", first)
		}
		// The server running it dies: its lease runs out.
		_, _ = d.Pool.Exec(ctx, `UPDATE task SET lease_until = now() - interval '1 second' WHERE id=$1`, id)
		second, _ := d.Claim(ctx, kinds, time.Minute)
		if second == nil || second.ID != id || second.Attempts != 2 {
			t.Fatalf("reclaim: %+v", second)
		}
		_ = d.FinishTask(ctx, id, true, "gave up")
	})

	t.Run("retry waits, release does not count", func(t *testing.T) {
		id, _ := d.Enqueue(ctx, db.NewTask{UserID: alice, Kind: "document", Subject: subject}, nil)
		task, _ := d.Claim(ctx, kinds, time.Minute)
		if err := d.RetryTask(ctx, task.ID, time.Hour, "busy"); err != nil {
			t.Fatal(err)
		}
		if again, _ := d.Claim(ctx, kinds, time.Minute); again != nil {
			t.Fatal("a retry must wait for its delay")
		}
		_, _ = d.Pool.Exec(ctx, `UPDATE task SET run_after = now() WHERE id=$1`, id)
		task, _ = d.Claim(ctx, kinds, time.Minute)
		if task == nil || task.Attempts != 2 {
			t.Fatalf("retry claim: %+v", task)
		}
		_ = d.ReleaseTask(ctx, task.ID) // clean shutdown
		task, _ = d.Claim(ctx, kinds, time.Minute)
		if task == nil || task.Attempts != 2 {
			t.Fatalf("a released task keeps its attempt count: %+v", task)
		}
		_ = d.FinishTask(ctx, id, false, "")
	})

	t.Run("other kinds are not claimed", func(t *testing.T) {
		id, _ := d.Enqueue(ctx, db.NewTask{UserID: alice, Kind: "journey", Subject: subject}, nil)
		if task, _ := d.Claim(ctx, kinds, time.Minute); task != nil {
			t.Fatalf("claimed a %s task as a document worker", task.Kind)
		}
		_ = d.FinishTask(ctx, id, false, "")
	})

	t.Run("finished tasks are purged after an hour", func(t *testing.T) {
		_, _ = d.Pool.Exec(ctx, `UPDATE task SET updated_at = now() - interval '2 hours'`)
		d.PurgeTasks(ctx)
		var n, events int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM task`).Scan(&n)
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM task_event`).Scan(&events)
		if n != 0 || events != 0 {
			t.Fatalf("left %d tasks and %d events", n, events)
		}
	})

	t.Run("marketing choice is stored", func(t *testing.T) {
		if err := d.AcceptTerms(ctx, alice, "v1", true); err != nil {
			t.Fatal(err)
		}
		var optIn bool
		var at *time.Time
		_ = d.Pool.QueryRow(ctx, `SELECT marketing_opt_in, marketing_updated_at FROM users WHERE id=$1`, alice).Scan(&optIn, &at)
		if !optIn || at == nil {
			t.Fatalf("opt-in at acceptance not stored: %v %v", optIn, at)
		}
		if err := d.SetMarketing(ctx, alice, false); err != nil {
			t.Fatal(err)
		}
		u, err := d.UserBySession(ctx, mustSession(t, d, alice))
		if err != nil || u.MarketingOptIn {
			t.Fatalf("opt-out not stored: %+v %v", u, err)
		}
		exp, err := d.ExportAll(ctx, alice)
		if err != nil || exp.Account.MarketingOptIn {
			t.Fatalf("export should show the stored choice: %v", err)
		}
	})

	t.Run("deleting a user deletes their tasks", func(t *testing.T) {
		_, _ = d.Enqueue(ctx, db.NewTask{UserID: bob, Kind: "document", Subject: subject, Input: []byte("x")}, map[string]any{"type": "progress"})
		if err := d.DeleteUser(ctx, bob); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM task WHERE user_id=$1`, bob).Scan(&n)
		if n != 0 {
			t.Fatal("tasks survived account deletion")
		}
	})
}

func mustSession(t *testing.T, d *db.DB, uid string) string {
	t.Helper()
	tok, _, err := d.CreateSession(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
