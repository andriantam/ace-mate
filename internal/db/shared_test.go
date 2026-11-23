package db_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model/modeltest"
)

// Shared prep: one task or HR question kept once per user and linked into several journeys.
func TestSharedPrep(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	idA, err := d.UpsertOAuthUser(ctx, "google", "sub-sa", "sa@example.com", "SA", "")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := d.UpsertOAuthUser(ctx, "google", "sub-sb", "sb@example.com", "SB", "")
	if err != nil {
		t.Fatal(err)
	}
	j := modeltest.Journey()
	job1, err := d.InsertJob(ctx, idA, db.NewJob{Title: "Frontend role", Journey: &j})
	if err != nil {
		t.Fatal(err)
	}
	job2, err := d.InsertJob(ctx, idA, db.NewJob{Title: "Second frontend role", Journey: &j})
	if err != nil {
		t.Fatal(err)
	}
	job3, err := d.InsertJob(ctx, idA, db.NewJob{Title: "Backend role", Journey: &j})
	if err != nil {
		t.Fatal(err)
	}

	// Progress from before the upgrade: ticked in job 1, and an unrelated tick in job 3.
	if _, err := d.SetProgress(ctx, idA, job1, "t1:0", true); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetProgress(ctx, idA, job3, "t2:1", true); err != nil {
		t.Fatal(err)
	}

	t.Run("jobs written before the upgrade are waiting to be linked", func(t *testing.T) {
		ids, err := d.UnsyncedJobs(ctx, idA)
		if err != nil || len(ids) != 3 {
			t.Fatalf("unsynced = %v, %v", ids, err)
		}
	})

	link := func(job string, links ...db.ShareLink) {
		t.Helper()
		if err := d.ApplyShareLinks(ctx, idA, job, links); err != nil {
			t.Fatal(err)
		}
	}
	link(job1,
		db.ShareLink{Ref: "t1:0", Kind: db.SharedTask, Key: "frontend/js-project-on-github", Label: "Push a JS project to GitHub"},
		db.ShareLink{Ref: "h1:q0", Kind: db.SharedHR, Key: "hr/greatest-weakness", Label: "What is your greatest weakness?"})
	link(job2,
		db.ShareLink{Ref: "t3:1", Kind: db.SharedTask, Key: "frontend/js-project-on-github", Label: "ignored: the key exists"},
		db.ShareLink{Ref: "h2:q4", Kind: db.SharedHR, Key: "hr/greatest-weakness", Label: "Weakness"})
	// The same exercise in a backend role is a different key, so it stays separate.
	link(job3, db.ShareLink{Ref: "t1:0", Kind: db.SharedTask, Key: "backend/js-project-on-github", Label: "Push a JS project to GitHub"})

	progress := func(job string) map[string]bool {
		t.Helper()
		j, err := d.GetJob(ctx, idA, job)
		if err != nil {
			t.Fatal(err)
		}
		return j.Progress
	}

	t.Run("linking carries an existing tick to the other journey, and keeps every old tick", func(t *testing.T) {
		if !progress(job2)["t3:1"] {
			t.Fatal("job 2 should have the shared task ticked")
		}
		if !progress(job1)["t1:0"] || !progress(job3)["t2:1"] {
			t.Fatal("existing ticks must be kept")
		}
		if progress(job3)["t1:0"] {
			t.Fatal("the backend task is a different item and must not be ticked")
		}
		if ids, _ := d.UnsyncedJobs(ctx, idA); len(ids) != 0 {
			t.Fatalf("all jobs are linked now, got %v", ids)
		}
	})

	t.Run("ticking a shared task ticks it in every linked journey", func(t *testing.T) {
		changed, err := d.SetProgress(ctx, idA, job2, "t3:1", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 2 {
			t.Fatalf("changed = %+v", changed)
		}
		if progress(job1)["t1:0"] || progress(job2)["t3:1"] {
			t.Fatal("unticking should reach both journeys")
		}
		if changed, err := d.SetProgress(ctx, idA, job1, "t2:0", true); err != nil || len(changed) != 1 {
			t.Fatalf("an unshared task changes only itself: %+v %v", changed, err)
		}
	})

	t.Run("a job lists what it shares with the others", func(t *testing.T) {
		refs, err := d.SharedRefs(ctx, idA, job1)
		if err != nil {
			t.Fatal(err)
		}
		r := refs["t1:0"]
		if r.Kind != db.SharedTask || r.Label != "Push a JS project to GitHub" || len(r.Others) != 1 || r.Others[0].JobID != job2 {
			t.Fatalf("t1:0 = %+v", r)
		}
		if h := refs["h1:q0"]; h.Kind != db.SharedHR || len(h.Others) != 1 {
			t.Fatalf("h1:q0 = %+v", h)
		}
	})

	var hrID, taskID string
	t.Run("the FAQ lists shared items with where they appear", func(t *testing.T) {
		items, err := d.ListShared(ctx, idA)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 3 {
			t.Fatalf("items = %+v", items)
		}
		for _, it := range items {
			switch it.Key {
			case "hr/greatest-weakness":
				hrID = it.ID
				if len(it.Links) != 2 {
					t.Fatalf("hr links = %+v", it.Links)
				}
			case "frontend/js-project-on-github":
				taskID = it.ID
			}
		}
		if hrID == "" || taskID == "" {
			t.Fatal("missing items")
		}
	})

	t.Run("ticking from the FAQ reaches the journeys", func(t *testing.T) {
		changed, err := d.SetSharedDone(ctx, idA, taskID, true)
		if err != nil || len(changed) != 2 {
			t.Fatalf("%+v %v", changed, err)
		}
		if !progress(job1)["t1:0"] || !progress(job2)["t3:1"] {
			t.Fatal("should be ticked in both")
		}
		if _, err := d.SetSharedDone(ctx, idA, hrID, true); !errors.Is(err, db.ErrNotFound) {
			t.Fatal("an HR question has no tick")
		}
	})

	t.Run("other user cannot see or change shared items", func(t *testing.T) {
		if refs, err := d.SharedRefs(ctx, idB, job1); err != nil || len(refs) != 0 {
			t.Fatalf("B saw %v %v", refs, err)
		}
		if items, err := d.ListShared(ctx, idB); err != nil || len(items) != 0 {
			t.Fatalf("B saw %v %v", items, err)
		}
		if err := d.SetSharedAnswer(ctx, idB, hrID, "hijacked"); !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("answer: %v", err)
		}
		if _, err := d.SetSharedDone(ctx, idB, taskID, false); !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("done: %v", err)
		}
		if err := d.ApplyShareLinks(ctx, idB, job1, []db.ShareLink{{Ref: "t1:1", Kind: db.SharedTask, Key: "general/x", Label: "x"}}); err != nil {
			t.Fatal(err) // the job isn't B's, so nothing happens
		}
		jobB, err := d.InsertJob(ctx, idB, db.NewJob{Title: "B"})
		if err != nil {
			t.Fatal(err)
		}
		err = d.WithUser(ctx, idB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO shared_link (job_id, ref, item_id, user_id) VALUES ($1,'t1:0',$2,$3)`, jobB, taskID, idB)
			return err
		})
		if err == nil {
			t.Fatal("B must not link A's item")
		}
		if refs, _ := d.SharedRefs(ctx, idA, job1); len(refs) != 2 {
			t.Fatalf("A's links changed: %v", refs)
		}
	})

	t.Run("an answer survives deleting the jobs that asked it", func(t *testing.T) {
		if err := d.SetSharedAnswer(ctx, idA, hrID, "I over-polish; I now time-box reviews."); err != nil {
			t.Fatal(err)
		}
		for _, j := range []string{job1, job2, job3} {
			if err := d.DeleteJob(ctx, idA, j); err != nil {
				t.Fatal(err)
			}
		}
		items, err := d.ListShared(ctx, idA)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 1 || items[0].ID != hrID || items[0].Answer == "" || len(items[0].Links) != 0 {
			t.Fatalf("items = %+v", items)
		}
	})

	t.Run("deleting the user removes shared items", func(t *testing.T) {
		if err := d.DeleteUser(ctx, idA); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM shared_item WHERE user_id=$1)+(SELECT count(*) FROM shared_link WHERE user_id=$1)`, idA).Scan(&n)
		if n != 0 {
			t.Fatalf("%d rows left", n)
		}
	})
}

// The upgrade as it happens on a real database: journeys stored encrypted (so the plaintext
// journey column is empty), and shared-prep tables left by a pre-release build of this
// feature (migration 0002_shared_prep.sql, labels and answers in plaintext). After the
// upgrade every job, journey and tick is intact, every job is offered for linking, and the
// pre-release labels and answers are encrypted and still readable.
func TestUpgradeToSharedPrepKeepsData(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	dir := t.TempDir()
	keys := testKeys(t)
	d, err := db.Open(ctx, url, dir, false, keys)
	if err != nil {
		t.Fatal(err)
	}
	uid, err := d.UpsertOAuthUser(ctx, "google", "sub-upgrade", "u@example.com", "U", "")
	if err != nil {
		t.Fatal(err)
	}
	j := modeltest.Journey()
	var jobs []string
	for i := 0; i < 6; i++ {
		id, err := d.InsertJob(ctx, uid, db.NewJob{Title: "Role " + strconv.Itoa(i), Notes: "notes", Journey: &j})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.SetProgress(ctx, uid, id, "t1:0", i%2 == 0); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, id)
	}
	var plain int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM job WHERE user_id=$1 AND journey IS NOT NULL`, uid).Scan(&plain)
	if plain != 0 {
		t.Fatalf("journeys should be stored encrypted only, %d are in plaintext", plain)
	}
	// What the pre-release build left: two jobs linked, plaintext label and answer, and
	// its migration recorded under its old name.
	var itemID string
	err = d.WithUser(ctx, uid, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO shared_item (user_id,kind,key,label,answer) VALUES ($1,'hr','hr/greatest-weakness','What is your greatest weakness?','I over-polish.') RETURNING id::text`, uid).Scan(&itemID); err != nil {
			return err
		}
		for _, job := range jobs[:2] {
			if _, err := tx.Exec(ctx, `INSERT INTO shared_link (job_id,ref,item_id,user_id) VALUES ($1,'h1:q0',$2,$3)`, job, itemID, uid); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE job SET shared_synced_at=now() WHERE id=$1`, job); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `
		ALTER TABLE shared_item DROP COLUMN label_enc, DROP COLUMN answer_enc;
		DELETE FROM _migrations WHERE name IN ('0006_shared_prep.sql', '0006_shared_prep.backfill');
		INSERT INTO _migrations (name) VALUES ('0002_shared_prep.sql')`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d, err = db.Open(ctx, url, dir, false, keys) // the upgrade
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)

	list, err := d.ListJobs(ctx, uid)
	if err != nil || len(list) != 6 {
		t.Fatalf("jobs after upgrade: %d %v", len(list), err)
	}
	for _, s := range list {
		if s.Journey == nil || len(s.Journey.Nodes) != len(j.Nodes) {
			t.Fatalf("job %q lost its journey in the list", s.Title)
		}
	}
	for i, id := range jobs {
		got, err := d.GetJob(ctx, uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "ready" || got.Journey == nil || len(got.Journey.Nodes) != len(j.Nodes) || got.Notes != "notes" {
			t.Fatalf("job %d changed: %+v", i, got)
		}
		if got.Progress["t1:0"] != (i%2 == 0) {
			t.Fatalf("job %d progress changed: %v", i, got.Progress)
		}
	}
	if ids, err := d.UnsyncedJobs(ctx, uid); err != nil || len(ids) != 4 {
		t.Fatalf("the four jobs not linked yet should wait to be linked: %v %v", ids, err)
	}
	items, err := d.ListShared(ctx, uid)
	if err != nil || len(items) != 1 || items[0].Label != "What is your greatest weakness?" || items[0].Answer != "I over-polish." || len(items[0].Links) != 2 {
		t.Fatalf("pre-release item after upgrade: %+v %v", items, err)
	}
	var label, answer string
	var sealed bool
	_ = d.Pool.QueryRow(ctx, `SELECT label, answer, label_enc IS NOT NULL AND answer_enc IS NOT NULL FROM shared_item WHERE id=$1`, itemID).Scan(&label, &answer, &sealed)
	if label != "" || answer != "" || !sealed {
		t.Fatalf("plaintext should be encrypted and cleared: label=%q answer=%q sealed=%v", label, answer, sealed)
	}
}

// Labels and answers are never written in plaintext.
func TestSharedPrepIsEncrypted(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	uid, err := d.UpsertOAuthUser(ctx, "google", "sub-enc", "e@example.com", "E", "")
	if err != nil {
		t.Fatal(err)
	}
	j := modeltest.Journey()
	job, err := d.InsertJob(ctx, uid, db.NewJob{Title: "Role", Journey: &j})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyShareLinks(ctx, uid, job, []db.ShareLink{{Ref: "h1:q0", Kind: db.SharedHR, Key: "hr/secret-question", Label: "A label to hide"}}); err != nil {
		t.Fatal(err)
	}
	items, _ := d.ListShared(ctx, uid)
	if len(items) != 1 || items[0].Label != "A label to hide" {
		t.Fatalf("items = %+v", items)
	}
	if err := d.SetSharedAnswer(ctx, uid, items[0].ID, "An answer to hide"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM shared_item WHERE position('to hide' in label || answer || encode(coalesce(label_enc,''),'escape') || encode(coalesce(answer_enc,''),'escape')) > 0`).Scan(&n)
	if n != 0 {
		t.Fatal("label or answer stored readable")
	}
	refs, _ := d.SharedRefs(ctx, uid, job)
	if refs["h1:q0"].Answer != "An answer to hide" {
		t.Fatalf("refs = %+v", refs)
	}
	if err := d.SetSharedAnswer(ctx, uid, items[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if items, _ := d.ListShared(ctx, uid); len(items) != 1 || items[0].Answer != "" {
		t.Fatalf("clearing an answer: %+v", items)
	}
}
