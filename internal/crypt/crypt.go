// Package crypt holds AceMate's encryption keys and the AES-256-GCM primitives built on them.
//
// Keys come in two layers (envelope encryption):
//   - master keys (key-encryption keys) live outside the database, in ENCRYPTION_KEYS;
//   - each user has a random data key, stored in the database only wrapped (encrypted) by
//     a master key.
//
// A copy of the database alone therefore can't decrypt anyone's data. Deleting a user's
// wrapped data key makes everything encrypted under it unreadable, wherever a copy lives.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// KeySize is the size of every key: AES-256.
const KeySize = 32

// sealVersion is the first byte of every sealed value, so the format can change later.
const sealVersion byte = 1

var ErrDecrypt = errors.New("crypt: cannot decrypt (wrong key, wrong owner or tampered data)")

// Keyring holds the master keys by version. New wraps always use the newest version; older
// versions are kept so data wrapped before a rotation can still be read and re-wrapped.
type Keyring struct {
	current uint32
	keys    map[uint32]cipher.AEAD
}

// ParseKeyring reads ENCRYPTION_KEYS: comma-separated "version:base64key" entries, each key
// 32 random bytes (for example from `openssl rand -base64 32`). The highest version is current.
func ParseKeyring(spec string) (*Keyring, error) {
	k := &Keyring{keys: map[uint32]cipher.AEAD{}}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		vs, b64, ok := strings.Cut(part, ":")
		if !ok {
			return nil, errors.New(`ENCRYPTION_KEYS: each entry must be "version:base64key"`)
		}
		v, err := strconv.ParseUint(strings.TrimSpace(vs), 10, 32)
		if err != nil || v == 0 {
			return nil, fmt.Errorf("ENCRYPTION_KEYS: version %q must be a whole number from 1", vs)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
		if err != nil {
			return nil, fmt.Errorf("ENCRYPTION_KEYS: key %d is not valid base64", v)
		}
		if err := k.add(uint32(v), raw); err != nil {
			return nil, err
		}
	}
	if len(k.keys) == 0 {
		return nil, errors.New("ENCRYPTION_KEYS is empty")
	}
	return k, nil
}

// NewKeyring makes a keyring with a single master key.
func NewKeyring(version uint32, key []byte) (*Keyring, error) {
	k := &Keyring{keys: map[uint32]cipher.AEAD{}}
	if version == 0 {
		return nil, errors.New("crypt: key versions start at 1")
	}
	return k, k.add(version, key)
}

func (k *Keyring) add(v uint32, key []byte) error {
	if len(key) != KeySize {
		return fmt.Errorf("ENCRYPTION_KEYS: key %d must be %d bytes, got %d", v, KeySize, len(key))
	}
	if _, dup := k.keys[v]; dup {
		return fmt.Errorf("ENCRYPTION_KEYS: version %d appears twice", v)
	}
	a, err := newAEAD(key)
	if err != nil {
		return err
	}
	k.keys[v] = a
	if v > k.current {
		k.current = v
	}
	return nil
}

// Current is the master-key version used for new wraps.
func (k *Keyring) Current() uint32 { return k.current }

// Versions lists the loaded master-key versions, oldest first.
func (k *Keyring) Versions() []uint32 {
	out := make([]uint32, 0, len(k.keys))
	for v := range k.keys {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// LoadKeyring returns the configured keyring. In production ENCRYPTION_KEYS is required.
// In development, without it, a key is generated once and kept in dataDir/encryption.key,
// next to the embedded database it protects, so local data survives restarts.
func LoadKeyring(spec, dataDir string, production bool) (*Keyring, error) {
	if strings.TrimSpace(spec) != "" {
		return ParseKeyring(spec)
	}
	if production {
		return nil, errors.New("APP_ENV=production requires ENCRYPTION_KEYS (generate one with: openssl rand -base64 32)")
	}
	path := filepath.Join(dataDir, "encryption.key")
	const hint = " (set ENCRYPTION_KEYS to 1:<output of: openssl rand -base64 32>)"
	if b, err := os.ReadFile(path); err == nil {
		return ParseKeyring(string(b))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("ENCRYPTION_KEYS is not set and the development key in %s can't be read: %w%s", dataDir, err, hint)
	}
	key, err := NewDataKey()
	if err != nil {
		return nil, err
	}
	line := "1:" + base64.StdEncoding.EncodeToString(key)
	err = os.MkdirAll(dataDir, 0o700)
	if err == nil {
		err = os.WriteFile(path, []byte(line+"\n"), 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEYS is not set and a development key can't be saved in %s: %w%s", dataDir, err, hint)
	}
	return ParseKeyring(line)
}

// NewDataKey returns a fresh random 256-bit key.
func NewDataKey() ([]byte, error) {
	b := make([]byte, KeySize)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// wrapAAD binds a wrapped key to its owner, so one user's wrapped key copied onto another
// user's row fails to unwrap instead of silently working.
func wrapAAD(userID string) string { return "acemate/user-key|" + userID }

// WrapUserKey encrypts a user's data key with the current master key.
func (k *Keyring) WrapUserKey(userID string, dek []byte) (version uint32, wrapped []byte, err error) {
	if len(dek) != KeySize {
		return 0, nil, errors.New("crypt: data key must be 32 bytes")
	}
	wrapped, err = seal(k.keys[k.current], wrapAAD(userID), dek)
	return k.current, wrapped, err
}

// UnwrapUserKey decrypts a user's data key wrapped under the given master-key version.
func (k *Keyring) UnwrapUserKey(userID string, version uint32, wrapped []byte) ([]byte, error) {
	a, ok := k.keys[version]
	if !ok {
		return nil, fmt.Errorf("crypt: master key version %d is not loaded (check ENCRYPTION_KEYS)", version)
	}
	dek, err := open(a, wrapAAD(userID), wrapped)
	if err != nil {
		return nil, err
	}
	if len(dek) != KeySize {
		return nil, ErrDecrypt
	}
	return dek, nil
}

// Seal encrypts plaintext with a data key. context names what the value is and whose it is
// (for example "job.notes|<job id>"): opening it under any other context fails.
func Seal(dek []byte, context string, plaintext []byte) ([]byte, error) {
	a, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	return seal(a, context, plaintext)
}

// Open decrypts a value made by Seal with the same data key and context.
func Open(dek []byte, context string, sealed []byte) ([]byte, error) {
	a, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	return open(a, context, sealed)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, errors.New("crypt: key must be 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Sealed layout: version byte | 12-byte random nonce | ciphertext and tag.
func seal(a cipher.AEAD, aad string, plaintext []byte) ([]byte, error) {
	out := make([]byte, 1+a.NonceSize(), 1+a.NonceSize()+len(plaintext)+a.Overhead())
	out[0] = sealVersion
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, err
	}
	return a.Seal(out, out[1:], plaintext, []byte(aad)), nil
}

func open(a cipher.AEAD, aad string, sealed []byte) ([]byte, error) {
	n := a.NonceSize()
	if len(sealed) < 1+n+a.Overhead() || sealed[0] != sealVersion {
		return nil, ErrDecrypt
	}
	pt, err := a.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(aad))
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}
