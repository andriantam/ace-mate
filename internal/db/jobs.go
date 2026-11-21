package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/model"
)

// ───────── jobs ─────────

type JobSummary struct {
	ID        string
	Title     string
	Company   string
	URL       string
	Status    string
	CreatedAt time.Time
	Journey   *model.Journey
	Progress  model.Progress
}

func (d *DB) ListJobs(ctx context.Context, userID string) ([]JobSummary, error) {
	var out []JobSummary
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, title, company, COALESCE(url,''), status, created_at, journey
			FROM job WHERE user_id=$1 ORDER BY created_at DESC`, userID)
		if err != nil {
			return err
		}
		idx := map[string]int{}
		for rows.Next() {
			var j JobSummary
			if err := rows.Scan(&j.ID, &j.Title, &j.Company, &j.URL, &j.Status, &j.CreatedAt, &j.Journey); err != nil {
				rows.Close()
				return err
			}
			j.Progress = model.Progress{}
			idx[j.ID] = len(out)
			out = append(out, j)
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}
		pr, err := tx.Query(ctx, `SELECT job_id::text, item, done FROM progress WHERE user_id=$1`, userID)
		if err != nil {
			return err
		}
		defer pr.Close()
		for pr.Next() {
			var id, item string
			var done bool
			if err := pr.Scan(&id, &item, &done); err != nil {
				return err
			}
			if i, ok := idx[id]; ok {
				out[i].Progress[item] = done
			}
		}
		return pr.Err()
	})
	return out, err
}

type Job struct {
	ID        string
	URL       string
	Notes     string
	JDText    string
	Title     string
	Company   string
	Status    string
	Error     string
	Journey   *model.Journey
	People    model.People
	Progress  model.Progress
	CreatedAt time.Time
}

func (d *DB) GetJob(ctx context.Context, userID, jobID string) (*Job, error) {
	var j Job
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id::text, COALESCE(url,''), notes, COALESCE(jd_text,''), title, company, status, COALESCE(error,''), journey, people, created_at
			FROM job WHERE id=$1 AND user_id=$2`, jobID, userID).
			Scan(&j.ID, &j.URL, &j.Notes, &j.JDText, &j.Title, &j.Company, &j.Status, &j.Error, &j.Journey, &j.People, &j.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		j.Progress = model.Progress{}
		rows, err := tx.Query(ctx, `SELECT item, done FROM progress WHERE job_id=$1 AND user_id=$2`, jobID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item string
			var done bool
			if err := rows.Scan(&item, &done); err != nil {
				return err
			}
			j.Progress[item] = done
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	if j.People == nil {
		j.People = model.People{}
	}
	return &j, nil
}

type NewJob struct {
	URL, Notes, JDText, Title, Company string
	Journey                            *model.Journey
}

func (d *DB) InsertJob(ctx context.Context, userID string, n NewJob) (string, error) {
	var id string
	status := "pending"
	if n.Journey != nil {
		status = "ready"
	}
	title := n.Title
	if title == "" {
		title = "Analysing…"
	}
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO job (user_id,url,notes,jd_text,title,company,journey,status)
			VALUES ($1,NULLIF($2,''),$3,NULLIF($4,''),$5,$6,$7,$8) RETURNING id::text`,
			userID, n.URL, n.Notes, n.JDText, title, n.Company, n.Journey, status).Scan(&id)
	})
	return id, err
}

func (d *DB) SetJobJD(ctx context.Context, userID, jobID, jd string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET jd_text=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID, jd)
		return err
	})
}

func (d *DB) MarkJobReady(ctx context.Context, userID, jobID string, j model.Journey) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET status='ready', error=NULL, journey=$3, title=$4, company=$5, updated_at=now() WHERE id=$1 AND user_id=$2`,
			jobID, userID, j, orDefault(j.Meta.Title, "Untitled role"), j.Meta.Company)
		return err
	})
}

// SaveJourneyPartial stores a journey that is still being written, so the user can start
// on the finished parts. The job stays pending.
func (d *DB) SaveJourneyPartial(ctx context.Context, userID, jobID string, j model.Journey) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET journey=$3, title=$4, company=$5, updated_at=now() WHERE id=$1 AND user_id=$2 AND status='pending'`,
			jobID, userID, j, orDefault(j.Meta.Title, "Untitled role"), j.Meta.Company)
		return err
	})
}

func (d *DB) MarkJobFailed(ctx context.Context, userID, jobID, msg string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET status='failed', error=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID, msg)
		return err
	})
}

func (d *DB) MarkJobPending(ctx context.Context, userID, jobID string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET status='pending', error=NULL, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID)
		return err
	})
}

func (d *DB) DeleteJob(ctx context.Context, userID, jobID string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM job WHERE id=$1 AND user_id=$2`, jobID, userID)
		return err
	})
}

func (d *DB) SetProgress(ctx context.Context, userID, jobID, item string, done bool) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		// The policy's WITH CHECK also verifies the job belongs to this user.
		_, err := tx.Exec(ctx, `
			INSERT INTO progress (job_id,user_id,item,done) VALUES ($1,$2,$3,$4)
			ON CONFLICT (job_id,item) DO UPDATE SET done=EXCLUDED.done, updated_at=now()`, jobID, userID, item, done)
		return err
	})
}

func (d *DB) SavePeople(ctx context.Context, userID, jobID string, p model.People) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET people=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID, p)
		return err
	})
}

// CountRecentGenerations counts journeys started in the last 24h, and those still pending.
func (d *DB) CountRecentGenerations(ctx context.Context, userID string) (n int, pending int, err error) {
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE created_at > now() - interval '24 hours'),
			       count(*) FILTER (WHERE status='pending')
			FROM job WHERE user_id=$1`, userID).Scan(&n, &pending)
	})
	return
}
