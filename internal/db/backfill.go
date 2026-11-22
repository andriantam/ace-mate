package db

// encryptExisting runs once, at the first start after column encryption was added: it
// encrypts every user's plaintext content into the *_enc columns and clears the plaintext.
// It is safe to interrupt and re-run (a row's encrypted value is never overwritten), and it
// records itself in _migrations when complete so later starts skip it.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

const backfillName = "0003_encrypt_columns.backfill"

func (d *DB) encryptExisting(ctx context.Context) error {
	var done bool
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM _migrations WHERE name=$1)`, backfillName).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	rows, err := d.Pool.Query(ctx, `SELECT id FROM users`)
	if err != nil {
		return err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	n := 0
	for _, u := range users {
		c, err := d.encryptUserRows(ctx, u)
		if err != nil {
			return err
		}
		n += c
	}
	c, err := d.encryptTaskRows(ctx)
	if err != nil {
		return err
	}
	n += c
	if _, err := d.Pool.Exec(ctx, `INSERT INTO _migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`, backfillName); err != nil {
		return err
	}
	slog.Info("encryption: encrypted existing user data", "rows", n)
	return nil
}

// sealRaw seals raw JSON from a jsonb column; SQL NULL, JSON null and {} stay NULL.
func sealRaw(s sealer, column, rowID string, raw []byte) ([]byte, error) {
	switch string(raw) {
	case "", "null", "{}":
		return nil, nil
	}
	return s.bytes(column, rowID, raw)
}

// encryptUserRows encrypts one user's profile, documents and jobs, under row-level security.
func (d *DB) encryptUserRows(ctx context.Context, userID string) (int, error) {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return 0, err
	}
	n := 0
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		// profile
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT data::text FROM profile WHERE user_id=$1 AND data IS NOT NULL`, userID).Scan(&raw)
		if err == nil {
			enc, err := sealRaw(s, "profile.data", userID, raw)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE profile SET data_enc=COALESCE(data_enc,$2), data=NULL WHERE user_id=$1`, userID, enc); err != nil {
				return err
			}
			n++
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// documents
		type docRow struct {
			id, name, url string
			extracted     []byte
			verification  []byte
		}
		rows, err := tx.Query(ctx, `SELECT id::text, name, COALESCE(url,''), extracted::text, verification::text FROM document
			WHERE user_id=$1 AND (name <> '' OR url IS NOT NULL OR extracted IS NOT NULL OR verification IS NOT NULL)`, userID)
		if err != nil {
			return err
		}
		docs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (docRow, error) {
			var x docRow
			return x, r.Scan(&x.id, &x.name, &x.url, &x.extracted, &x.verification)
		})
		if err != nil {
			return err
		}
		for _, x := range docs {
			var e [4][]byte
			if e[0], err = s.text("document.name", x.id, x.name); err != nil {
				return err
			}
			if e[1], err = s.text("document.url", x.id, x.url); err != nil {
				return err
			}
			if e[2], err = sealRaw(s, "document.extracted", x.id, x.extracted); err != nil {
				return err
			}
			if e[3], err = sealRaw(s, "document.verification", x.id, x.verification); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE document SET
				name_enc=COALESCE(name_enc,$2), url_enc=COALESCE(url_enc,$3),
				extracted_enc=COALESCE(extracted_enc,$4), verification_enc=COALESCE(verification_enc,$5),
				name='', url=NULL, extracted=NULL, verification=NULL WHERE id=$1`, x.id, e[0], e[1], e[2], e[3]); err != nil {
				return err
			}
			n++
		}

		// jobs
		type jobRow struct {
			id, url, notes, jd string
			journey, people    []byte
		}
		rows, err = tx.Query(ctx, `SELECT id::text, COALESCE(url,''), COALESCE(notes,''), COALESCE(jd_text,''), journey::text, people::text FROM job
			WHERE user_id=$1 AND (url IS NOT NULL OR notes <> '' OR jd_text IS NOT NULL OR journey IS NOT NULL OR people <> '{}'::jsonb)`, userID)
		if err != nil {
			return err
		}
		jobs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (jobRow, error) {
			var x jobRow
			return x, r.Scan(&x.id, &x.url, &x.notes, &x.jd, &x.journey, &x.people)
		})
		if err != nil {
			return err
		}
		for _, x := range jobs {
			var e [5][]byte
			for i, f := range []struct{ col, v string }{{"job.url", x.url}, {"job.notes", x.notes}, {"job.jd_text", x.jd}} {
				if e[i], err = s.text(f.col, x.id, f.v); err != nil {
					return err
				}
			}
			if e[3], err = sealRaw(s, "job.journey", x.id, x.journey); err != nil {
				return err
			}
			if e[4], err = sealRaw(s, "job.people", x.id, x.people); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE job SET
				url_enc=COALESCE(url_enc,$2), notes_enc=COALESCE(notes_enc,$3), jd_enc=COALESCE(jd_enc,$4),
				journey_enc=COALESCE(journey_enc,$5), people_enc=COALESCE(people_enc,$6),
				url=NULL, notes='', jd_text=NULL, journey=NULL, people='{}' WHERE id=$1`, x.id, e[0], e[1], e[2], e[3], e[4]); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// encryptTaskRows encrypts queued uploads and progress messages. These tables are only
// used by the server's own code, so they are read without a user context.
func (d *DB) encryptTaskRows(ctx context.Context) (int, error) {
	type row struct {
		id, userID string
		input      []byte
	}
	rows, err := d.Pool.Query(ctx, `SELECT id::text, user_id, input FROM task WHERE input IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	tasks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		return x, r.Scan(&x.id, &x.userID, &x.input)
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, x := range tasks {
		s, err := d.sealerFor(ctx, x.userID)
		if err != nil {
			return n, err
		}
		enc, err := s.bytes("task.input", x.id, x.input)
		if err != nil {
			return n, err
		}
		if _, err := d.Pool.Exec(ctx, `UPDATE task SET input_enc=COALESCE(input_enc,$2), input=NULL WHERE id=$1`, x.id, enc); err != nil {
			return n, err
		}
		n++
	}

	type ev struct {
		id             int64
		taskID, userID string
		data           json.RawMessage
	}
	rows, err = d.Pool.Query(ctx, `SELECT e.id, e.task_id::text, t.user_id, e.data FROM task_event e JOIN task t ON t.id=e.task_id WHERE e.data IS NOT NULL`)
	if err != nil {
		return n, err
	}
	events, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ev, error) {
		var x ev
		return x, r.Scan(&x.id, &x.taskID, &x.userID, &x.data)
	})
	if err != nil {
		return n, err
	}
	for _, x := range events {
		s, err := d.sealerFor(ctx, x.userID)
		if err != nil {
			return n, err
		}
		enc, err := s.bytes("task_event.data", x.taskID, x.data)
		if err != nil {
			return n, err
		}
		if _, err := d.Pool.Exec(ctx, `UPDATE task_event SET data_enc=COALESCE(data_enc,$2), data=NULL WHERE id=$1`, x.id, enc); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
