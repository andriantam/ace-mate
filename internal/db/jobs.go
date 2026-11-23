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
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []JobSummary
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, title, company, status, created_at, url_enc, journey_enc, COALESCE(url,''), journey
			FROM job WHERE user_id=$1 ORDER BY created_at DESC`, userID)
		if err != nil {
			return err
		}
		idx := map[string]int{}
		for rows.Next() {
			var j JobSummary
			var urlEnc, journeyEnc []byte
			if err := rows.Scan(&j.ID, &j.Title, &j.Company, &j.Status, &j.CreatedAt, &urlEnc, &journeyEnc, &j.URL, &j.Journey); err != nil {
				rows.Close()
				return err
			}
			if err := openJobURLJourney(s, j.ID, urlEnc, journeyEnc, &j.URL, &j.Journey); err != nil {
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
	Engine    string // the model the user picked for this journey; "" is the site default
	Journey   *model.Journey
	People    model.People
	Progress  model.Progress
	CreatedAt time.Time
}

func (d *DB) GetJob(ctx context.Context, userID, jobID string) (*Job, error) {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	var j Job
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		var urlEnc, notesEnc, jdEnc, journeyEnc, peopleEnc []byte
		// The plaintext columns are only read for rows written before encryption, until the
		// startup pass encrypts them.
		err := tx.QueryRow(ctx, `
			SELECT id::text, title, company, status, COALESCE(error,''), engine, created_at,
			       url_enc, notes_enc, jd_enc, journey_enc, people_enc,
			       COALESCE(url,''), COALESCE(notes,''), COALESCE(jd_text,''), journey, people
			FROM job WHERE id=$1 AND user_id=$2`, jobID, userID).
			Scan(&j.ID, &j.Title, &j.Company, &j.Status, &j.Error, &j.Engine, &j.CreatedAt,
				&urlEnc, &notesEnc, &jdEnc, &journeyEnc, &peopleEnc,
				&j.URL, &j.Notes, &j.JDText, &j.Journey, &j.People)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := openJobURLJourney(s, j.ID, urlEnc, journeyEnc, &j.URL, &j.Journey); err != nil {
			return err
		}
		for _, f := range []struct {
			col string
			enc []byte
			dst *string
		}{{"job.notes", notesEnc, &j.Notes}, {"job.jd_text", jdEnc, &j.JDText}} {
			if f.enc != nil {
				v, err := s.openText(f.col, j.ID, f.enc)
				if err != nil {
					return err
				}
				*f.dst = v
			}
		}
		if peopleEnc != nil {
			j.People = model.People{}
			if _, err := s.openJSON("job.people", j.ID, peopleEnc, &j.People); err != nil {
				return err
			}
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
	URL, Notes, JDText, Title, Company, Engine string
	Journey                                    *model.Journey
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
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return "", err
	}
	id = newUUID()
	var enc [4][]byte
	for i, f := range []struct{ col, v string }{{"job.url", n.URL}, {"job.notes", n.Notes}, {"job.jd_text", n.JDText}} {
		if enc[i], err = s.text(f.col, id, f.v); err != nil {
			return "", err
		}
	}
	if enc[3], err = s.json("job.journey", id, n.Journey); err != nil {
		return "", err
	}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO job (id,user_id,title,company,status,url_enc,notes_enc,jd_enc,journey_enc,engine)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			id, userID, title, n.Company, status, enc[0], enc[1], enc[2], enc[3], n.Engine)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// openJobURLJourney fills the job's url and journey from their encrypted columns, where set.
func openJobURLJourney(s sealer, jobID string, urlEnc, journeyEnc []byte, url *string, journey **model.Journey) error {
	if urlEnc != nil {
		v, err := s.openText("job.url", jobID, urlEnc)
		if err != nil {
			return err
		}
		*url = v
	}
	if journeyEnc != nil {
		var j model.Journey
		if _, err := s.openJSON("job.journey", jobID, journeyEnc, &j); err != nil {
			return err
		}
		*journey = &j
	}
	return nil
}

func (d *DB) SetJobJD(ctx context.Context, userID, jobID, jd string) error {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	enc, err := s.text("job.jd_text", jobID, jd)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET jd_text=NULL, jd_enc=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID, enc)
		return err
	})
}

func (d *DB) sealJourney(ctx context.Context, userID, jobID string, j model.Journey) ([]byte, error) {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.json("job.journey", jobID, j)
}

func (d *DB) MarkJobReady(ctx context.Context, userID, jobID string, j model.Journey) error {
	enc, err := d.sealJourney(ctx, userID, jobID, j)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET status='ready', error=NULL, journey=NULL, journey_enc=$3, title=$4, company=$5, updated_at=now() WHERE id=$1 AND user_id=$2`,
			jobID, userID, enc, orDefault(j.Meta.Title, "Untitled role"), j.Meta.Company)
		return err
	})
}

// SaveJourneyPartial stores a journey that is still being written, so the user can start
// on the finished parts. The job stays pending.
func (d *DB) SaveJourneyPartial(ctx context.Context, userID, jobID string, j model.Journey) error {
	enc, err := d.sealJourney(ctx, userID, jobID, j)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET journey=NULL, journey_enc=$3, title=$4, company=$5, updated_at=now() WHERE id=$1 AND user_id=$2 AND status='pending'`,
			jobID, userID, enc, orDefault(j.Meta.Title, "Untitled role"), j.Meta.Company)
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
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	enc, err := s.json("job.people", jobID, p)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET people='{}', people_enc=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID, enc)
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
