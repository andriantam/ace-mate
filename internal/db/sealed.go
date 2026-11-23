package db

// Column encryption. Every column with a user's personal content is stored sealed with
// that user's data key (see keys.go and migrations/0003_encrypt_columns.sql).
//
// Each value is bound to "table.column|owner|row", so a sealed value moved to another row,
// another column or another user fails to decrypt instead of being read in the wrong place.

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/andriantam/ace-mate/internal/crypt"
)

// sealer encrypts and decrypts one user's values.
type sealer struct {
	dek    []byte
	userID string
}

func (d *DB) sealerFor(ctx context.Context, userID string) (sealer, error) {
	dek, err := d.UserKey(ctx, userID)
	if err != nil {
		return sealer{}, fmt.Errorf("loading the user's encryption key: %w", err)
	}
	return sealer{dek: dek, userID: userID}, nil
}

func (s sealer) context(column, rowID string) string {
	return "acemate/" + column + "|" + s.userID + "|" + rowID
}

// bytes seals raw bytes. Empty input is stored as NULL.
func (s sealer) bytes(column, rowID string, b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	return crypt.Seal(s.dek, s.context(column, rowID), b)
}

func (s sealer) openBytes(column, rowID string, sealed []byte) ([]byte, error) {
	if sealed == nil {
		return nil, nil
	}
	b, err := crypt.Open(s.dek, s.context(column, rowID), sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: %w", column, err)
	}
	return b, nil
}

func (s sealer) text(column, rowID, v string) ([]byte, error) {
	return s.bytes(column, rowID, []byte(v))
}

func (s sealer) openText(column, rowID string, sealed []byte) (string, error) {
	b, err := s.openBytes(column, rowID, sealed)
	return string(b), err
}

// json seals v as JSON. A nil pointer, map or slice is stored as NULL.
func (s sealer) json(column, rowID string, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	return s.bytes(column, rowID, b)
}

// openJSON decrypts into out. It returns false (and leaves out alone) for NULL.
func (s sealer) openJSON(column, rowID string, sealed []byte, out any) (bool, error) {
	b, err := s.openBytes(column, rowID, sealed)
	if err != nil || b == nil {
		return false, err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return false, fmt.Errorf("decoding %s: %w", column, err)
	}
	return true, nil
}

// newUUID returns a random (version 4) UUID, so a row's id is known before it is inserted
// and its values can be bound to it.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
