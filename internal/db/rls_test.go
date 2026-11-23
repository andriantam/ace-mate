package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/db"
	"github.com/andriantam/ace-mate/internal/model"
)

// Proves tenant isolation at the database layer: user B can't read, modify, delete or
// attach to user A's rows, even with queries that omit WHERE user_id.
// Needs the embedded Postgres (downloaded on first run) or TEST_DATABASE_URL.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	url := os.Getenv("TEST_DATABASE_URL")
	dir := t.TempDir()
	d, err := db.Open(ctx, url, dir, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

func TestRowLevelSecurity(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	idA, err := d.UpsertOAuthUser(ctx, "google", "sub-a", "a@example.com", "A", "")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := d.UpsertOAuthUser(ctx, "google", "sub-b", "b@example.com", "B", "")
	if err != nil {
		t.Fatal(err)
	}

	jobA, err := d.InsertJob(ctx, idA, db.NewJob{Title: "A's job", Notes: "secret notes"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertProfile(ctx, idA, model.Profile{Headline: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetProgress(ctx, idA, jobA, "t1:0", true); err != nil {
		t.Fatal(err)
	}
	docA, err := d.InsertDocument(ctx, idA, "link", "github.com/a", "https://github.com/a")
	if err != nil {
		t.Fatal(err)
	}

	count := func(userID, table string) int {
		var n int
		err := d.WithUser(ctx, userID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n) // deliberately no WHERE user_id
		})
		if err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return n
	}

	t.Run("owner reads own rows", func(t *testing.T) {
		if count(idA, "job") != 1 || count(idA, "profile") != 1 || count(idA, "progress") != 1 || count(idA, "document") != 1 {
			t.Fatal("owner should see their rows")
		}
	})
	t.Run("other user sees nothing, even without a WHERE clause", func(t *testing.T) {
		if n := count(idB, "job") + count(idB, "profile") + count(idB, "progress") + count(idB, "document"); n != 0 {
			t.Fatalf("user B saw %d rows", n)
		}
	})
	t.Run("other user cannot read, finish or delete a document", func(t *testing.T) {
		if _, err := d.GetDocument(ctx, idB, docA); !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("get: want ErrNotFound, got %v", err)
		}
		if err := d.DeleteDocument(ctx, idB, docA); !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("delete: want ErrNotFound, got %v", err)
		}
		_ = d.FinishDocument(ctx, idB, docA, "hijacked", model.Profile{Headline: "B"}, nil)
		if doc, err := d.GetDocument(ctx, idA, docA); err != nil || doc.Status != "processing" || doc.Name != "github.com/a" {
			t.Fatalf("A's document changed: %+v %v", doc, err)
		}
	})
	t.Run("other user cannot fetch by id", func(t *testing.T) {
		if _, err := d.GetJob(ctx, idB, jobA); !errors.Is(err, db.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})
	t.Run("other user cannot update or delete", func(t *testing.T) {
		_ = d.WithUser(ctx, idB, func(tx pgx.Tx) error {
			ct, err := tx.Exec(ctx, `UPDATE job SET title='pwned'`)
			if err != nil || ct.RowsAffected() != 0 {
				t.Fatalf("update affected %v rows (err %v)", ct.RowsAffected(), err)
			}
			ct, err = tx.Exec(ctx, `DELETE FROM job`)
			if err != nil || ct.RowsAffected() != 0 {
				t.Fatalf("delete affected %v rows (err %v)", ct.RowsAffected(), err)
			}
			return nil
		})
		j, err := d.GetJob(ctx, idA, jobA)
		if err != nil || j.Title != "A's job" {
			t.Fatalf("job changed: %v %v", j, err)
		}
	})
	t.Run("other user cannot insert rows owned by someone else", func(t *testing.T) {
		err := d.WithUser(ctx, idB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO job (user_id,title) VALUES ($1,'spoof')`, idA)
			return err
		})
		if err == nil {
			t.Fatal("insert for another user should fail")
		}
	})
	t.Run("other user cannot attach progress to someone else's job", func(t *testing.T) {
		if _, err := d.SetProgress(ctx, idB, jobA, "t1:1", true); err == nil {
			t.Fatal("expected policy violation")
		}
	})
	t.Run("other user cannot hand their rows to someone else", func(t *testing.T) {
		jobB, err := d.InsertJob(ctx, idB, db.NewJob{Title: "B"})
		if err != nil {
			t.Fatal(err)
		}
		err = d.WithUser(ctx, idB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE job SET user_id=$2 WHERE id=$1`, jobB, idA)
			return err
		})
		if err == nil {
			t.Fatal("changing ownership should fail")
		}
	})
	t.Run("no user context means no access", func(t *testing.T) {
		tx, err := d.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE app_user`); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM job`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("count=%d err=%v", n, err)
		}
	})
	t.Run("app role cannot read identity tables", func(t *testing.T) {
		err := d.WithUser(ctx, idA, func(tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&n)
		})
		if err == nil {
			t.Fatal("app role must not read sessions")
		}
	})
	t.Run("deleting the user cascades all their data", func(t *testing.T) {
		if err := d.DeleteUser(ctx, idA); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = d.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM job WHERE user_id=$1)+(SELECT count(*) FROM profile WHERE user_id=$1)+(SELECT count(*) FROM progress WHERE user_id=$1)`, idA).Scan(&n)
		if n != 0 {
			t.Fatalf("%d rows left", n)
		}
	})
}

func TestIdentityAndSessions(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	a1, _ := d.UpsertOAuthUser(ctx, "google", "same-sub", "x@example.com", "X", "")
	a2, _ := d.UpsertOAuthUser(ctx, "google", "same-sub", "x@example.com", "X renamed", "")
	if a1 != a2 {
		t.Fatal("same identity must map to the same user")
	}
	// The same email at another provider is a different account: no takeover by email.
	b, _ := d.UpsertOAuthUser(ctx, "okta", "other-sub", "x@example.com", "X", "")
	if b == a1 {
		t.Fatal("different provider identities must not be merged by email")
	}

	tok, _, err := d.CreateSession(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
	u, err := d.UserBySession(ctx, tok)
	if err != nil || u.ID != a1 {
		t.Fatalf("session lookup: %v %v", u, err)
	}
	var stored []byte
	_ = d.Pool.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE user_id=$1`, a1).Scan(&stored)
	if string(stored) == tok || len(stored) != 32 {
		t.Fatal("only a hash of the token may be stored")
	}
	if _, err := d.UserBySession(ctx, "not-a-token"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	_, _ = d.Pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 minute'`)
	if _, err := d.UserBySession(ctx, tok); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("expired sessions must not authenticate")
	}
	tok2, _, _ := d.CreateSession(ctx, a1)
	_ = d.DeleteSession(ctx, tok2)
	if _, err := d.UserBySession(ctx, tok2); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("logout must revoke the session")
	}
	_ = time.Now
}
