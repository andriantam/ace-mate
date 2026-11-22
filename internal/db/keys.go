package db

// Per-user data keys (see internal/crypt and migrations/0002_user_keys.sql).
//
// Every user gets a random data key when their account is created. The database holds it
// only wrapped by a master key from ENCRYPTION_KEYS. The server unwraps it whenever it works
// on that user's data, in a request or in a background task, and keeps unwrapped keys in
// memory for a short while. Deleting the account deletes the key.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/andriantam/ace-mate/internal/crypt"
)

// keyCacheTTL bounds how long an unwrapped key stays in memory after its last load.
const keyCacheTTL = time.Hour

type cachedKey struct {
	dek     []byte
	expires time.Time
}

type keyCache struct {
	mu sync.Mutex
	m  map[string]cachedKey
}

func (c *keyCache) get(userID string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[userID]
	if !ok || time.Now().After(e.expires) {
		delete(c.m, userID)
		return nil, false
	}
	return e.dek, true
}

func (c *keyCache) put(userID string, dek []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cachedKey{}
	}
	c.m[userID] = cachedKey{dek: dek, expires: time.Now().Add(keyCacheTTL)}
}

func (c *keyCache) evict(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[userID]; ok {
		clear(e.dek)
		delete(c.m, userID)
	}
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// insertUserKey creates and stores a new wrapped key for userID, unless one exists.
// q is the pool, or the transaction that creates the user.
func (d *DB) insertUserKey(ctx context.Context, q execer, userID string) error {
	dek, err := crypt.NewDataKey()
	if err != nil {
		return err
	}
	v, wrapped, err := d.keys.WrapUserKey(userID, dek)
	clear(dek)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO user_key (user_id, kek_version, wrapped) VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO NOTHING`, userID, int64(v), wrapped)
	return err
}

// UserKey returns the user's unwrapped data key, creating it if the user has none yet.
// Callers must not modify or keep the returned slice beyond their own use.
func (d *DB) UserKey(ctx context.Context, userID string) ([]byte, error) {
	if userID == "" {
		return nil, errors.New("db: missing user id")
	}
	if dek, ok := d.keyCache.get(userID); ok {
		return dek, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		var v int64
		var wrapped []byte
		err := d.Pool.QueryRow(ctx, `SELECT kek_version, wrapped FROM user_key WHERE user_id=$1`, userID).Scan(&v, &wrapped)
		if errors.Is(err, pgx.ErrNoRows) && attempt == 0 {
			// An account created before keys existed. The users foreign key makes this fail
			// for a deleted user instead of creating an orphan key.
			if err := d.insertUserKey(ctx, d.Pool, userID); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		dek, err := d.keys.UnwrapUserKey(userID, uint32(v), wrapped)
		if err != nil {
			return nil, err
		}
		d.keyCache.put(userID, dek)
		return dek, nil
	}
	return nil, ErrNotFound
}

// prepareUserKeys runs at startup: it gives every user without a key a new one, and
// re-wraps keys made under an older master key with the current one, so an old master key
// can be removed from ENCRYPTION_KEYS once this has run.
func (d *DB) prepareUserKeys(ctx context.Context) error {
	rows, err := d.Pool.Query(ctx, `SELECT u.id FROM users u LEFT JOIN user_key k ON k.user_id=u.id WHERE k.user_id IS NULL`)
	if err != nil {
		return err
	}
	missing, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range missing {
		if err := d.insertUserKey(ctx, d.Pool, id); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		slog.Info("encryption: created data keys for existing users", "count", len(missing))
	}

	type old struct {
		id      string
		v       int64
		wrapped []byte
	}
	rows, err = d.Pool.Query(ctx, `SELECT user_id, kek_version, wrapped FROM user_key WHERE kek_version <> $1`, int64(d.keys.Current()))
	if err != nil {
		return err
	}
	stale, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (old, error) {
		var o old
		return o, r.Scan(&o.id, &o.v, &o.wrapped)
	})
	if err != nil {
		return err
	}
	for _, o := range stale {
		dek, err := d.keys.UnwrapUserKey(o.id, uint32(o.v), o.wrapped)
		if err != nil {
			return err
		}
		v, wrapped, err := d.keys.WrapUserKey(o.id, dek)
		clear(dek)
		if err != nil {
			return err
		}
		// Only replace what we read, in case another instance rotated it meanwhile.
		if _, err := d.Pool.Exec(ctx, `UPDATE user_key SET kek_version=$2, wrapped=$3, rotated_at=now()
			WHERE user_id=$1 AND kek_version=$4`, o.id, int64(v), wrapped, o.v); err != nil {
			return err
		}
	}
	if len(stale) > 0 {
		slog.Info("encryption: re-wrapped data keys with the current master key", "count", len(stale), "version", d.keys.Current())
	}
	return nil
}
