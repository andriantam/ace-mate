package db_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/andriantam/ace-mate/internal/crypt"
	"github.com/andriantam/ace-mate/internal/db"
)

// Every user gets a data key with their account; only a wrapped copy is stored; deleting
// the account deletes the key.
func TestUserKeys(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	alice, err := d.UpsertOAuthUser(ctx, "google", "sub-alice", "alice@example.com", "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	bob, _ := d.UpsertOAuthUser(ctx, "google", "sub-bob", "bob@example.com", "Bob", "")

	var version int
	var wrapped []byte
	if err := d.Pool.QueryRow(ctx, `SELECT kek_version, wrapped FROM user_key WHERE user_id=$1`, alice).Scan(&version, &wrapped); err != nil {
		t.Fatalf("a new user must get a key with their account: %v", err)
	}
	if version != 1 {
		t.Fatalf("kek_version = %d, want 1", version)
	}
	ka, err := d.UserKey(ctx, alice)
	if err != nil || len(ka) != crypt.KeySize {
		t.Fatalf("UserKey: %v", err)
	}
	if bytes.Contains(wrapped, ka) {
		t.Fatal("the database must hold only the wrapped key")
	}
	again, _ := d.UpsertOAuthUser(ctx, "google", "sub-alice", "alice@example.com", "Alice", "")
	if k2, _ := d.UserKey(ctx, again); !bytes.Equal(k2, ka) {
		t.Fatal("signing in again must not replace the key")
	}
	kb, _ := d.UserKey(ctx, bob)
	if bytes.Equal(ka, kb) {
		t.Fatal("each user must have their own key")
	}

	t.Run("app role cannot read keys", func(t *testing.T) {
		err := d.WithUser(ctx, alice, func(tx pgxTx) error {
			var n int
			return tx.QueryRow(ctx, `SELECT count(*) FROM user_key`).Scan(&n)
		})
		if err == nil {
			t.Fatal("queries running as a user must not read user_key")
		}
	})

	t.Run("delete removes the key", func(t *testing.T) {
		sealed, _ := crypt.Seal(ka, "test|alice", []byte("resume"))
		if err := d.DeleteUser(ctx, alice); err != nil {
			t.Fatal(err)
		}
		var n int
		_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM user_key WHERE user_id=$1`, alice).Scan(&n)
		if n != 0 {
			t.Fatal("the key must be deleted with the account")
		}
		if _, err := d.UserKey(ctx, alice); err == nil {
			t.Fatal("a deleted user's key must not be served from memory or recreated")
		}
		// Ciphertext left elsewhere (a backup) can't be opened without the key; the sealed
		// value still opens only with the exact key we held before deletion.
		if _, err := crypt.Open(kb, "test|alice", sealed); err == nil {
			t.Fatal("another key must not open it")
		}
		if k, _ := d.UserKey(ctx, bob); !bytes.Equal(k, kb) {
			t.Fatal("other users' keys are untouched")
		}
	})
}

// Users created before keys existed get one at startup, and keys move to a new master key.
func TestKeyBackfillAndRotation(t *testing.T) {
	ctx := context.Background()
	url, stop, err := db.StartEmbeddedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)

	k1, _ := crypt.NewKeyring(1, bytes.Repeat([]byte{1}, crypt.KeySize))
	d, err := db.Open(ctx, url, "", false, k1)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := d.UpsertOAuthUser(ctx, "google", "sub-old", "old@example.com", "Old", "")
	before, _ := d.UserKey(ctx, id)
	before = bytes.Clone(before)
	legacy := "legacy-user"
	if _, err := d.Pool.Exec(ctx, `INSERT INTO users (id) VALUES ($1)`, legacy); err != nil {
		t.Fatal(err)
	}
	d.Close()

	both, err := crypt.ParseKeyring("2:" + b64(bytes.Repeat([]byte{2}, crypt.KeySize)) + ",1:" + b64(bytes.Repeat([]byte{1}, crypt.KeySize)))
	if err != nil {
		t.Fatal(err)
	}
	d, err = db.Open(ctx, url, "", false, both)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM user_key WHERE kek_version <> 2`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d keys still wrapped by the old master key", n)
	}
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM user_key WHERE user_id=$1`, legacy).Scan(&n)
	if n != 1 {
		t.Fatal("an existing user without a key must get one at startup")
	}
	after, err := d.UserKey(ctx, id)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("rotation must keep the same data key: %v", err)
	}
	d.Close()

	// Once rotated, the old master key can be removed.
	k2, _ := crypt.NewKeyring(2, bytes.Repeat([]byte{2}, crypt.KeySize))
	d, err = db.Open(ctx, url, "", false, k2)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if k, err := d.UserKey(ctx, id); err != nil || !bytes.Equal(k, before) {
		t.Fatalf("after removing the old master key: %v", err)
	}
}

type pgxTx = pgx.Tx

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
