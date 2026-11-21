// Package db owns the Postgres connection, migrations and row-level-security helper.
package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andriantam/ace-mate/migrations"
)

type DB struct {
	Pool       *pgxpool.Pool
	roleSwitch bool // true when the restricted app_user role is available
	stop       func()
}

// Open connects to DATABASE_URL, or in development starts an embedded Postgres under dataDir.
func Open(ctx context.Context, databaseURL, dataDir string, production bool) (*DB, error) {
	stop := func() {}
	if databaseURL == "" {
		if production {
			return nil, errors.New("DATABASE_URL is required in production")
		}
		url, s, err := startEmbedded(dataDir)
		if err != nil {
			return nil, fmt.Errorf("starting embedded postgres: %w", err)
		}
		databaseURL, stop = url, s
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		stop()
		return nil, err
	}
	d := &DB{Pool: pool, stop: stop}
	if err := pool.Ping(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("cannot reach the database: %w", err)
	}
	if err := d.migrate(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	if err := d.checkRLS(ctx); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
	d.stop()
}

var migName = regexp.MustCompile(`^[\w.-]+\.sql$`)

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS _migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	rows, err := d.Pool.Query(ctx, `SELECT name FROM _migrations`)
	if err != nil {
		return err
	}
	applied := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		applied[n] = true
	}
	rows.Close()

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if migName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if applied[n] {
			continue
		}
		body, err := migrations.FS.ReadFile(n)
		if err != nil {
			return err
		}
		// Advisory lock so concurrent instances don't race.
		if err := d.applyOne(ctx, n, string(body)); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
		slog.Info("migration applied", "name", n)
	}
	return nil
}

func (d *DB) applyOne(ctx context.Context, name, body string) error {
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(7241001)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7241001)`)

	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM _migrations WHERE name = $1)`, name).Scan(&exists); err != nil || exists {
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Simple protocol so a file may hold many statements.
	if _, err := tx.Exec(ctx, body, pgx.QueryExecModeSimpleProtocol); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO _migrations (name) VALUES ($1)`, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// checkRLS decides how isolation is enforced and refuses to run if it can't be.
func (d *DB) checkRLS(ctx context.Context) error {
	var super, bypass, hasRole bool
	if err := d.Pool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		return err
	}
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user')`).Scan(&hasRole); err != nil {
		return err
	}
	d.roleSwitch = hasRole
	if (super || bypass) && !hasRole {
		return errors.New("the database user bypasses row-level security and the restricted app_user role is missing; connect as a non-superuser or allow role creation")
	}
	if hasRole {
		slog.Info("row-level security: switching to restricted role app_user per transaction")
	} else {
		slog.Warn("row-level security: app_user role unavailable; relying on FORCE ROW LEVEL SECURITY for the owner role")
	}
	return nil
}

// WithUser runs fn as userID under row-level security. Every query on user data goes
// through here; the policies scope reads and writes to that user's rows.
func (d *DB) WithUser(ctx context.Context, userID string, fn func(tx pgx.Tx) error) error {
	return WithUser(ctx, d.Pool, d.roleSwitch, userID, fn)
}

// Beginner is satisfied by *pgxpool.Pool.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

func WithUser(ctx context.Context, b Beginner, roleSwitch bool, userID string, fn func(tx pgx.Tx) error) error {
	if userID == "" {
		return errors.New("db: missing user id")
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if roleSwitch {
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE app_user`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, userID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── embedded postgres (development and tests) ──

func freePort() (uint32, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return uint32(l.Addr().(*net.TCPAddr).Port), nil
}

func startEmbedded(dataDir string) (string, func(), error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", nil, err
	}
	port, err := freePort()
	if err != nil {
		return "", nil, err
	}
	cfg := embeddedpostgres.DefaultConfig().
		Port(port).
		DataPath(filepath.Join(abs, "postgres")).
		RuntimePath(filepath.Join(abs, "postgres-runtime")).
		BinariesPath(filepath.Join(abs, "postgres-bin")).
		Database("acemate").
		StartTimeout(60 * time.Second)
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", nil, err
	}
	pg := embeddedpostgres.NewDatabase(cfg)
	if err := pg.Start(); err != nil {
		return "", nil, err
	}
	url := "postgres://postgres:postgres@127.0.0.1:" + strconv.Itoa(int(port)) + "/acemate?sslmode=disable"
	return url, func() { _ = pg.Stop() }, nil
}

// StartEmbeddedForTest starts a throwaway embedded Postgres in dir.
func StartEmbeddedForTest(dir string) (url string, stop func(), err error) { return startEmbedded(dir) }
