package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model"
)

// secret marks every piece of personal content in these tests; it must never be readable
// in the database.
const secret = "ZEBRA-7731"

// rawTables returns every row of the user tables as text, the way someone with a copy of
// the database would see them.
func rawTables(t *testing.T, d *db.DB, userID string) string {
	t.Helper()
	ctx := context.Background()
	var b strings.Builder
	read := func(q pgx.Rows, err error) {
		if err != nil {
			t.Fatal(err)
		}
		defer q.Close()
		for q.Next() {
			var s string
			if err := q.Scan(&s); err != nil {
				t.Fatal(err)
			}
			b.WriteString(s)
		}
	}
	err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
		for _, tbl := range []string{"profile", "document", "job"} {
			read(tx.Query(ctx, `SELECT row_to_json(x)::text FROM `+tbl+` x`))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	read(d.Pool.Query(ctx, `SELECT row_to_json(x)::text FROM task x`))
	read(d.Pool.Query(ctx, `SELECT row_to_json(x)::text FROM task_event x`))
	return b.String()
}

func journeyWith(s string) *model.Journey {
	return &model.Journey{Meta: model.JobMeta{Title: "Engineer", Company: "Acme", Summary: s},
		Fit: model.Fit{Match: 70, Gaps: []string{s}}, Nodes: []model.Node{}}
}

func TestColumnsAreEncrypted(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	u, _ := d.UpsertOAuthUser(ctx, "google", "sub-enc", "enc@example.com", "Enc", "")

	if err := d.UpsertProfile(ctx, u, model.Profile{Headline: "Headline " + secret}); err != nil {
		t.Fatal(err)
	}
	doc, err := d.InsertDocument(ctx, u, "link", "cv-"+secret+".pdf", "https://example.com/"+secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishDocument(ctx, u, doc, "", model.Profile{Headline: secret}, map[string]string{"note": secret}); err != nil {
		t.Fatal(err)
	}
	job, err := d.InsertJob(ctx, u, db.NewJob{URL: "https://jobs.example.com/" + secret, Notes: "notes " + secret, JDText: "jd " + secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SaveJourneyPartial(ctx, u, job, *journeyWith(secret)); err != nil {
		t.Fatal(err)
	}
	if err := d.SavePeople(ctx, u, job, model.People{"n1": {{Name: "Person " + secret}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Enqueue(ctx, db.NewTask{UserID: u, Kind: "document", Subject: doc, Input: []byte("file " + secret)},
		map[string]any{"type": "progress", "detail": secret}); err != nil {
		t.Fatal(err)
	}

	if raw := rawTables(t, d, u); strings.Contains(raw, secret) {
		t.Fatalf("plaintext found in the database:\n%s", raw)
	}

	// Everything still reads back through the app.
	p, _ := d.GetProfile(ctx, u)
	if p == nil || p.Data.Headline != "Headline "+secret {
		t.Fatalf("profile: %+v", p)
	}
	docs, _ := d.ListDocuments(ctx, u)
	if len(docs) != 1 || docs[0].Name != "cv-"+secret+".pdf" || docs[0].URL != "https://example.com/"+secret ||
		docs[0].Extracted == nil || docs[0].Extracted.Headline != secret || !strings.Contains(string(docs[0].Verification), secret) {
		t.Fatalf("documents: %+v", docs)
	}
	j, err := d.GetJob(ctx, u, job)
	if err != nil {
		t.Fatal(err)
	}
	if j.URL != "https://jobs.example.com/"+secret || j.Notes != "notes "+secret || j.JDText != "jd "+secret ||
		j.Journey == nil || j.Journey.Meta.Summary != secret || j.People["n1"][0].Name != "Person "+secret {
		t.Fatalf("job: %+v", j)
	}
	if j.Title != "Engineer" || j.Company != "Acme" {
		t.Fatal("title and company stay readable for the jobs list")
	}
	list, _ := d.ListJobs(ctx, u)
	if len(list) != 1 || list[0].Journey == nil || list[0].URL != j.URL {
		t.Fatalf("job list: %+v", list)
	}
	task, err := d.Claim(ctx, []string{"document"}, 60e9)
	if err != nil || task == nil || string(task.Input) != "file "+secret {
		t.Fatalf("claim: %+v %v", task, err)
	}
	pr, _ := d.Progress(ctx, u, doc, 0)
	if len(pr.Events) != 1 || !strings.Contains(string(pr.Events[0]), secret) {
		t.Fatalf("progress: %+v", pr)
	}
	if err := d.Emit(ctx, task.ID, map[string]any{"type": "done", "detail": secret}); err != nil {
		t.Fatal(err)
	}
	if pr, _ = d.Progress(ctx, u, doc, pr.Next); len(pr.Events) != 1 {
		t.Fatalf("emitted event: %+v", pr)
	}
	if raw := rawTables(t, d, u); strings.Contains(raw, secret) {
		t.Fatal("emitted events must be encrypted too")
	}

	t.Run("a value moved to another row does not decrypt", func(t *testing.T) {
		other, _ := d.InsertJob(ctx, u, db.NewJob{Title: "Other"})
		err := d.WithUser(ctx, u, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE job SET notes_enc=(SELECT notes_enc FROM job WHERE id=$1) WHERE id=$2`, job, other)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.GetJob(ctx, u, other); err == nil {
			t.Fatal("a copied value must fail to decrypt")
		}
	})
}

// Data written before encryption is encrypted once at startup and still reads back.
func TestExistingDataIsEncryptedAtStartup(t *testing.T) {
	ctx := context.Background()
	url, stop, err := db.StartEmbeddedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	keys := testKeys(t)
	d, err := db.Open(ctx, url, "", false, keys)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := d.UpsertOAuthUser(ctx, "google", "sub-old", "old@example.com", "Old", "")

	// Rows as an earlier version wrote them: plaintext only.
	var docID, jobID string
	err = d.WithUser(ctx, u, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO profile (user_id,data) VALUES ($1,$2)`, u, model.Profile{Headline: secret}); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO document (user_id,kind,name,url,status,extracted) VALUES ($1,'link',$2,$3,'ready',$4) RETURNING id`,
			u, "doc "+secret, "https://example.com/"+secret, model.Profile{Headline: secret}).Scan(&docID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO job (user_id,url,notes,jd_text,title,company,status,journey,people) VALUES ($1,$2,$3,$4,'Engineer','Acme','ready',$5,$6) RETURNING id`,
			u, "https://jobs.example.com/"+secret, "notes "+secret, "jd "+secret, journeyWith(secret), model.People{"n1": {{Name: secret}}}).Scan(&jobID)
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := d.Pool.QueryRow(ctx, `INSERT INTO task (user_id,kind,subject,input) VALUES ($1,'document',$2,$3) RETURNING id`, u, docID, []byte(secret)).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO task_event (task_id,data) VALUES ($1,$2)`, taskID, map[string]any{"detail": secret}); err != nil {
		t.Fatal(err)
	}
	// Plaintext rows still read before the startup pass.
	if p, _ := d.GetProfile(ctx, u); p == nil || p.Data.Headline != secret {
		t.Fatal("plaintext rows must read until they are encrypted")
	}
	if _, err := d.Pool.Exec(ctx, `DELETE FROM _migrations WHERE name='0003_encrypt_columns.backfill'`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d, err = db.Open(ctx, url, "", false, keys)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if raw := rawTables(t, d, u); strings.Contains(raw, secret) {
		t.Fatalf("plaintext left after startup:\n%s", raw)
	}
	if p, _ := d.GetProfile(ctx, u); p == nil || p.Data.Headline != secret {
		t.Fatal("profile lost")
	}
	docs, _ := d.ListDocuments(ctx, u)
	if len(docs) != 1 || docs[0].Name != "doc "+secret || docs[0].Extracted == nil || docs[0].Extracted.Headline != secret {
		t.Fatalf("documents: %+v", docs)
	}
	j, err := d.GetJob(ctx, u, jobID)
	if err != nil || j.Notes != "notes "+secret || j.JDText != "jd "+secret || j.Journey == nil || j.Journey.Meta.Summary != secret || j.People["n1"][0].Name != secret {
		t.Fatalf("job: %+v %v", j, err)
	}
	if pr, _ := d.Progress(ctx, u, docID, 0); len(pr.Events) != 1 || !strings.Contains(string(pr.Events[0]), secret) {
		t.Fatalf("progress: %+v", pr)
	}
	task, _ := d.Claim(ctx, []string{"document"}, 60e9)
	if task == nil || string(task.Input) != secret {
		t.Fatalf("task: %+v", task)
	}
}
