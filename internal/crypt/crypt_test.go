package crypt

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, KeySize) }

func spec(entries ...string) string { return strings.Join(entries, ",") }

func entry(v string, b byte) string { return v + ":" + base64.StdEncoding.EncodeToString(key(b)) }

func TestWrapUnwrap(t *testing.T) {
	k, err := ParseKeyring(entry("1", 1))
	if err != nil {
		t.Fatal(err)
	}
	dek, _ := NewDataKey()
	v, wrapped, err := k.WrapUserKey("alice", dek)
	if err != nil || v != 1 {
		t.Fatalf("wrap: v=%d err=%v", v, err)
	}
	if bytes.Contains(wrapped, dek) {
		t.Fatal("wrapped key must not contain the plain key")
	}
	got, err := k.UnwrapUserKey("alice", v, wrapped)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	// Bound to its owner: the same bytes under another user id don't unwrap.
	if _, err := k.UnwrapUserKey("bob", v, wrapped); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other user: %v", err)
	}
	// Tampering is detected.
	bad := append([]byte(nil), wrapped...)
	bad[len(bad)-1] ^= 1
	if _, err := k.UnwrapUserKey("alice", v, bad); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered: %v", err)
	}
	// Another master key can't unwrap it.
	other, _ := ParseKeyring(entry("1", 2))
	if _, err := other.UnwrapUserKey("alice", v, wrapped); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong master: %v", err)
	}
}

func TestRotation(t *testing.T) {
	old, _ := ParseKeyring(entry("1", 1))
	dek, _ := NewDataKey()
	_, wrapped, _ := old.WrapUserKey("alice", dek)

	k, err := ParseKeyring(spec(entry("2", 2), entry("1", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if k.Current() != 2 {
		t.Fatalf("current = %d, want the highest version", k.Current())
	}
	got, err := k.UnwrapUserKey("alice", 1, wrapped)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("old version must still unwrap: %v", err)
	}
	if v, _, _ := k.WrapUserKey("alice", dek); v != 2 {
		t.Fatalf("new wraps use version %d, want 2", v)
	}
	if _, err := k.UnwrapUserKey("alice", 3, wrapped); err == nil {
		t.Fatal("unknown version must fail")
	}
}

func TestSealOpen(t *testing.T) {
	dek, _ := NewDataKey()
	msg := []byte("senior backend engineer, Go and Postgres")
	s1, err := Seal(dek, "job.notes|j1", msg)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := Seal(dek, "job.notes|j1", msg)
	if bytes.Equal(s1, s2) {
		t.Fatal("each seal must use a fresh nonce")
	}
	got, err := Open(dek, "job.notes|j1", s1)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("open: %v", err)
	}
	if _, err := Open(dek, "job.notes|j2", s1); !errors.Is(err, ErrDecrypt) {
		t.Fatal("a value moved to another row must not open")
	}
	other, _ := NewDataKey()
	if _, err := Open(other, "job.notes|j1", s1); !errors.Is(err, ErrDecrypt) {
		t.Fatal("another user's key must not open it")
	}
	if _, err := Open(dek, "x", []byte{1, 2}); !errors.Is(err, ErrDecrypt) {
		t.Fatal("short input must fail cleanly")
	}
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		"",
		"nokey",
		"0:" + base64.StdEncoding.EncodeToString(key(1)),
		"x:" + base64.StdEncoding.EncodeToString(key(1)),
		"1:not base64!",
		"1:" + base64.StdEncoding.EncodeToString([]byte("short")),
		spec(entry("1", 1), entry("1", 2)),
	} {
		if _, err := ParseKeyring(s); err == nil {
			t.Errorf("ParseKeyring(%q) should fail", s)
		}
	}
}

func TestLoadKeyring(t *testing.T) {
	if _, err := LoadKeyring("", t.TempDir(), true); err == nil {
		t.Fatal("production without ENCRYPTION_KEYS must fail")
	}
	dir := t.TempDir()
	k1, err := LoadKeyring("", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "encryption.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode %v, want 0600", info.Mode().Perm())
	}
	dek, _ := NewDataKey()
	v, w, _ := k1.WrapUserKey("u", dek)
	k2, err := LoadKeyring("", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k2.UnwrapUserKey("u", v, w); err != nil {
		t.Fatal("the development key must survive a restart")
	}
	// Where no key can be kept, the error says how to fix it.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyring("", blocker, false); err == nil || !strings.Contains(err.Error(), "set ENCRYPTION_KEYS") {
		t.Fatalf("unwritable data dir: %v", err)
	}
	// An explicit setting wins over the file.
	k3, _ := LoadKeyring(entry("7", 9), dir, false)
	if k3.Current() != 7 {
		t.Fatal("ENCRYPTION_KEYS must take precedence")
	}
}
