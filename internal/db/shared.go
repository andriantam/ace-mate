package db

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// ───────── prep shared across jobs ─────────
//
// A shared_item is one task or HR question that several of a user's journeys ask for. Each
// place it appears is a shared_link (job, ref). Ticking a linked task ticks the same task in
// every linked journey, by writing each journey's own progress row, so readiness scores keep
// working from the progress table alone.

const (
	SharedTask = "task"
	SharedHR   = "hr"
)

// The label and answer of a shared item are personal content: they are stored sealed in
// label_enc and answer_enc (see migrations/0006_shared_prep.sql). Plaintext in label and
// answer is only read from rows a pre-release build wrote, until encryptSharedItems runs.

const (
	colSharedLabel  = "shared_item.label"
	colSharedAnswer = "shared_item.answer"
)

// openShared returns an item's label and answer, decrypting them where they are sealed.
func (s sealer) openShared(id, label string, labelEnc []byte, answer string, answerEnc []byte) (string, string, error) {
	if labelEnc != nil {
		v, err := s.openText(colSharedLabel, id, labelEnc)
		if err != nil {
			return "", "", err
		}
		label = v
	}
	if answerEnc != nil {
		v, err := s.openText(colSharedAnswer, id, answerEnc)
		if err != nil {
			return "", "", err
		}
		answer = v
	}
	return label, answer, nil
}

// ShareLink places one task or HR question of a journey under a shared key.
type ShareLink struct {
	Ref   string // "<node>:<index>" for a task, "<node>:q<index>" for an HR question
	Kind  string // SharedTask or SharedHR
	Key   string
	Label string
}

// CatalogueItem is an existing shared item, as shown to the model that links a new journey.
type CatalogueItem struct {
	Kind  string   `json:"kind"`
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Jobs  []string `json:"jobs"` // titles of the jobs it appears in
}

// SharedCatalogue lists the user's shared items that still appear in at least one job other
// than exceptJob (the job being linked, whose old links are about to be replaced).
func (d *DB) SharedCatalogue(ctx context.Context, userID, exceptJob string) ([]CatalogueItem, error) {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []CatalogueItem{}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.id::text, s.kind, s.key, s.label, s.label_enc, array_agg(DISTINCT j.title ORDER BY j.title)
			FROM shared_item s
			JOIN shared_link l ON l.item_id = s.id
			JOIN job j ON j.id = l.job_id
			WHERE s.user_id = $1 AND l.job_id <> $2::uuid
			GROUP BY s.id ORDER BY s.kind, s.key`, userID, exceptJob)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (CatalogueItem, error) {
			var c CatalogueItem
			var id string
			var labelEnc []byte
			if err := r.Scan(&id, &c.Kind, &c.Key, &c.Label, &labelEnc, &c.Jobs); err != nil {
				return c, err
			}
			c.Label, _, err = s.openShared(id, c.Label, labelEnc, "", nil)
			return c, err
		})
		return err
	})
	return out, err
}

// ApplyShareLinks replaces a job's links with links, creating shared items for new keys.
// Linking a task merges ticks: if the task is done anywhere it's linked, it becomes done
// everywhere it's linked. It never unticks anything, so no progress is lost. It also
// records that the job has been linked.
func (d *DB) ApplyShareLinks(ctx context.Context, userID, jobID string, links []ShareLink) error {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		// One linking at a time per user, so two jobs finishing together agree on new keys.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7241002, hashtext($1))`, userID); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job WHERE id=$1 AND user_id=$2)`, jobID, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return nil // deleted meanwhile
		}
		if _, err := tx.Exec(ctx, `DELETE FROM shared_link WHERE job_id=$1 AND user_id=$2`, jobID, userID); err != nil {
			return err
		}
		touched := map[string]bool{}
		for _, l := range links {
			// A new item's id is chosen here so its label can be sealed to it. For a key the
			// user already has, the existing item (and its label) is kept.
			newID := newUUID()
			labelEnc, err := s.text(colSharedLabel, newID, l.Label)
			if err != nil {
				return err
			}
			var itemID string
			err = tx.QueryRow(ctx, `
				INSERT INTO shared_item (id, user_id, kind, key, label_enc) VALUES ($1,$2,$3,$4,$5)
				ON CONFLICT (user_id, kind, key) DO UPDATE SET updated_at = shared_item.updated_at
				RETURNING id::text`, newID, userID, l.Kind, l.Key, labelEnc).Scan(&itemID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO shared_link (job_id, ref, item_id, user_id) VALUES ($1,$2,$3,$4)
				ON CONFLICT (job_id, ref) DO UPDATE SET item_id = EXCLUDED.item_id`, jobID, l.Ref, itemID, userID); err != nil {
				return err
			}
			if l.Kind == SharedTask {
				touched[itemID] = true
			}
		}
		for id := range touched {
			// Done if it is already done, or ticked in any journey it's linked to.
			var done bool
			if err := tx.QueryRow(ctx, `
				SELECT s.done OR EXISTS (
					SELECT 1 FROM shared_link l JOIN progress p ON p.job_id = l.job_id AND p.item = l.ref
					WHERE l.item_id = s.id AND p.done)
				FROM shared_item s WHERE s.id = $1`, id).Scan(&done); err != nil {
				return err
			}
			if done {
				if err := setSharedDone(ctx, tx, userID, id, true); err != nil {
					return err
				}
			}
		}
		_, err := tx.Exec(ctx, `UPDATE job SET shared_synced_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID)
		return err
	})
}

// JobRef is one place a shared item appears.
type JobRef struct {
	JobID string `json:"jobId"`
	Title string `json:"title"`
	Ref   string `json:"ref"`
}

// SharedRef describes the shared item behind one ref of a job.
type SharedRef struct {
	ItemID string   `json:"id"`
	Kind   string   `json:"kind"`
	Label  string   `json:"label"`
	Answer string   `json:"answer,omitempty"`
	Others []JobRef `json:"others"` // the other jobs it appears in
}

// SharedRefs returns the shared items behind a job's tasks and HR questions, keyed by ref.
func (d *DB) SharedRefs(ctx context.Context, userID, jobID string) (map[string]SharedRef, error) {
	sl, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := map[string]SharedRef{}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT l.ref, s.id::text, s.kind, s.label, s.label_enc, s.answer, s.answer_enc,
			       COALESCE(json_agg(json_build_object('jobId', o.job_id::text, 'title', j.title, 'ref', o.ref)
			                ORDER BY j.created_at) FILTER (WHERE o.job_id IS NOT NULL), '[]')
			FROM shared_link l
			JOIN shared_item s ON s.id = l.item_id
			LEFT JOIN shared_link o ON o.item_id = s.id AND o.job_id <> l.job_id
			LEFT JOIN job j ON j.id = o.job_id
			WHERE l.job_id = $1 AND l.user_id = $2
			GROUP BY l.ref, s.id`, jobID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref string
			var s SharedRef
			var labelEnc, answerEnc []byte
			if err := rows.Scan(&ref, &s.ItemID, &s.Kind, &s.Label, &labelEnc, &s.Answer, &answerEnc, &s.Others); err != nil {
				return err
			}
			if s.Label, s.Answer, err = sl.openShared(s.ItemID, s.Label, labelEnc, s.Answer, answerEnc); err != nil {
				return err
			}
			// A job can hold the same item twice; list each other job once.
			s.Others = uniqueJobs(s.Others)
			out[ref] = s
		}
		return rows.Err()
	})
	return out, err
}

func uniqueJobs(in []JobRef) []JobRef {
	seen := map[string]bool{}
	out := []JobRef{}
	for _, j := range in {
		if !seen[j.JobID] {
			seen[j.JobID] = true
			out = append(out, j)
		}
	}
	return out
}

// Ticked is a progress item that changed, in some job.
type Ticked struct {
	JobID string `json:"jobId"`
	Item  string `json:"item"`
}

// SetProgress ticks or unticks an item of a journey. If it is a shared task, the same task
// is ticked in every journey it's linked to. It returns every item that changed.
func (d *DB) SetProgress(ctx context.Context, userID, jobID, item string, done bool) ([]Ticked, error) {
	changed := []Ticked{{jobID, item}}
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		// The policy's WITH CHECK also verifies the job belongs to this user.
		if _, err := tx.Exec(ctx, `
			INSERT INTO progress (job_id,user_id,item,done) VALUES ($1,$2,$3,$4)
			ON CONFLICT (job_id,item) DO UPDATE SET done=EXCLUDED.done, updated_at=now()`, jobID, userID, item, done); err != nil {
			return err
		}
		var itemID string
		err := tx.QueryRow(ctx, `
			SELECT s.id::text FROM shared_link l JOIN shared_item s ON s.id = l.item_id
			WHERE l.job_id=$1 AND l.ref=$2 AND s.kind='task'`, jobID, item).Scan(&itemID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := setSharedDone(ctx, tx, userID, itemID, done); err != nil {
			return err
		}
		changed, err = linksOf(ctx, tx, itemID)
		return err
	})
	return changed, err
}

// SetSharedDone ticks or unticks a shared task in every journey it's linked to.
func (d *DB) SetSharedDone(ctx context.Context, userID, itemID string, done bool) ([]Ticked, error) {
	var changed []Ticked
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		var kind string
		err := tx.QueryRow(ctx, `SELECT kind FROM shared_item WHERE id=$1 AND user_id=$2`, itemID, userID).Scan(&kind)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != SharedTask) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := setSharedDone(ctx, tx, userID, itemID, done); err != nil {
			return err
		}
		changed, err = linksOf(ctx, tx, itemID)
		return err
	})
	return changed, err
}

func setSharedDone(ctx context.Context, tx pgx.Tx, userID, itemID string, done bool) error {
	if _, err := tx.Exec(ctx, `UPDATE shared_item SET done=$2, updated_at=now() WHERE id=$1`, itemID, done); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO progress (job_id, user_id, item, done)
		SELECT job_id, $2, ref, $3 FROM shared_link WHERE item_id = $1
		ON CONFLICT (job_id, item) DO UPDATE SET done=EXCLUDED.done, updated_at=now()`, itemID, userID, done)
	return err
}

func linksOf(ctx context.Context, tx pgx.Tx, itemID string) ([]Ticked, error) {
	rows, err := tx.Query(ctx, `SELECT job_id::text, ref FROM shared_link WHERE item_id=$1 ORDER BY job_id, ref`, itemID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Ticked, error) {
		var t Ticked
		err := r.Scan(&t.JobID, &t.Item)
		return t, err
	})
}

// SetSharedAnswer saves the user's own answer to a shared HR question.
func (d *DB) SetSharedAnswer(ctx context.Context, userID, itemID, answer string) error {
	s, err := d.sealerFor(ctx, userID)
	if err != nil {
		return err
	}
	enc, err := s.text(colSharedAnswer, itemID, answer) // an empty answer is stored as NULL
	if err != nil {
		return err
	}
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE shared_item SET answer='', answer_enc=$3, updated_at=now() WHERE id=$1 AND user_id=$2 AND kind='hr'`, itemID, userID, enc)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
}

// SharedItem is one shared item and where it appears, for the profile's FAQ.
type SharedItem struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Key       string    `json:"key"`
	Label     string    `json:"label"`
	Done      bool      `json:"done"`
	Answer    string    `json:"answer"`
	UpdatedAt time.Time `json:"updatedAt"`
	Links     []JobRef  `json:"links"`
}

// ListShared returns the user's shared items that appear in a job, and HR questions the user
// answered even if the jobs that asked them have since been deleted.
func (d *DB) ListShared(ctx context.Context, userID string) ([]SharedItem, error) {
	sl, err := d.sealerFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := []SharedItem{}
	err = d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT s.id::text, s.kind, s.key, s.label, s.label_enc, s.done, s.answer, s.answer_enc, s.updated_at,
			       COALESCE(json_agg(json_build_object('jobId', l.job_id::text, 'title', j.title, 'ref', l.ref)
			                ORDER BY j.created_at, l.ref) FILTER (WHERE l.job_id IS NOT NULL), '[]')
			FROM shared_item s
			LEFT JOIN shared_link l ON l.item_id = s.id
			LEFT JOIN job j ON j.id = l.job_id
			WHERE s.user_id = $1
			GROUP BY s.id
			HAVING count(l.job_id) > 0 OR s.answer <> '' OR s.answer_enc IS NOT NULL
			ORDER BY count(l.job_id) DESC, s.key`, userID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (SharedItem, error) {
			var s SharedItem
			var labelEnc, answerEnc []byte
			if err := r.Scan(&s.ID, &s.Kind, &s.Key, &s.Label, &labelEnc, &s.Done, &s.Answer, &answerEnc, &s.UpdatedAt, &s.Links); err != nil {
				return s, err
			}
			s.Label, s.Answer, err = sl.openShared(s.ID, s.Label, labelEnc, s.Answer, answerEnc)
			return s, err
		})
		return err
	})
	return out, err
}

// UnsyncedJobs lists the user's ready jobs whose journeys haven't been linked yet.
func (d *DB) UnsyncedJobs(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text FROM job
			WHERE user_id=$1 AND status='ready' AND shared_synced_at IS NULL
			  AND (journey_enc IS NOT NULL OR journey IS NOT NULL) -- encrypted since 0003, or not yet
			ORDER BY created_at`, userID)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return ids, err
}

// MarkShareSynced records that a job was compared, even when nothing could be linked, so
// it isn't tried again at every start. The user can ask for another pass (ResetShareSync).
func (d *DB) MarkShareSynced(ctx context.Context, userID, jobID string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET shared_synced_at=now() WHERE id=$1 AND user_id=$2`, jobID, userID)
		return err
	})
}

// ResetShareSync marks all the user's jobs as not yet linked, for a fresh pass.
func (d *DB) ResetShareSync(ctx context.Context, userID string) error {
	return d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE job SET shared_synced_at=NULL WHERE user_id=$1`, userID)
		return err
	})
}

// UserIDs lists every user. Used to find work left over from before an upgrade.
func (d *DB) UserIDs(ctx context.Context) ([]string, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// OpenTask reports whether a task of kind on subject is queued or running.
func (d *DB) OpenTask(ctx context.Context, kind, subject string) (bool, error) {
	var ok bool
	err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM task WHERE kind=$1 AND subject=$2 AND status IN ('queued','running'))`,
		kind, subject).Scan(&ok)
	return ok, err
}

const sharedBackfillName = "0006_shared_prep.backfill"

// encryptSharedItems seals labels and answers that a pre-release build of shared prep
// stored in plaintext, and clears the plaintext. It runs once (recorded in _migrations);
// it is safe to interrupt and re-run, and a sealed value is never overwritten. From this
// version on, labels and answers are only ever written sealed.
func (d *DB) encryptSharedItems(ctx context.Context) error {
	var done bool
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM _migrations WHERE name=$1)`, sharedBackfillName).Scan(&done); err != nil || done {
		return err
	}
	users, err := d.UserIDs(ctx)
	if err != nil {
		return err
	}
	n := 0
	for _, uid := range users {
		s, err := d.sealerFor(ctx, uid)
		if err != nil {
			return err
		}
		err = d.WithUser(ctx, uid, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id::text, label, answer FROM shared_item
				WHERE user_id=$1 AND ((label <> '' AND label_enc IS NULL) OR (answer <> '' AND answer_enc IS NULL))`, uid)
			if err != nil {
				return err
			}
			type row struct{ id, label, answer string }
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
				var x row
				return x, r.Scan(&x.id, &x.label, &x.answer)
			})
			if err != nil {
				return err
			}
			for _, x := range list {
				le, err := s.text(colSharedLabel, x.id, x.label)
				if err != nil {
					return err
				}
				ae, err := s.text(colSharedAnswer, x.id, x.answer)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE shared_item SET
					label_enc  = COALESCE(label_enc, $2),  label  = '',
					answer_enc = COALESCE(answer_enc, $3), answer = ''
					WHERE id=$1`, x.id, le, ae); err != nil {
					return err
				}
				n++
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO _migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`, sharedBackfillName); err != nil {
		return err
	}
	if n > 0 {
		slog.Info("encryption: encrypted shared prep saved in plaintext", "items", n)
	}
	return nil
}
