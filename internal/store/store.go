// Package store wraps database/sql for SQLite (single node) and PostgreSQL
// (cluster) behind one small API. Queries are written with ? placeholders and
// rewritten for PostgreSQL.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/breefercp/breefercp/migrations"
)

// ErrNotFound is returned when a row lookup matches nothing.
var ErrNotFound = errors.New("not found")

// Store is the database handle used by all services.
type Store struct {
	DB     *sql.DB
	driver string
}

// Open connects and applies pending migrations.
func Open(ctx context.Context, driver, dsn string) (*Store, error) {
	var (
		db  *sql.DB
		err error
	)
	switch driver {
	case "sqlite":
		if dir := filepath.Dir(dsn); dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, err
			}
		}
		q := "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
		db, err = sql.Open("sqlite", "file:"+filepath.ToSlash(dsn)+q)
		if err == nil {
			// SQLite serialises writers; one connection avoids SQLITE_BUSY.
			db.SetMaxOpenConns(1)
		}
	case "postgres":
		db, err = sql.Open("pgx", dsn)
		if err == nil {
			db.SetMaxOpenConns(25)
			db.SetConnMaxLifetime(30 * time.Minute)
		}
	default:
		return nil, fmt.Errorf("unsupported driver %q", driver)
	}
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	s := &Store{DB: db, driver: driver}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the connection pool.
func (s *Store) Close() error { return s.DB.Close() }

// Driver reports "sqlite" or "postgres".
func (s *Store) Driver() string { return s.driver }

// Rebind converts ? placeholders to $n for PostgreSQL.
func (s *Store) Rebind(q string) string {
	if s.driver != "postgres" {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

// Exec runs a statement.
func (s *Store) Exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.DB.ExecContext(ctx, s.Rebind(q), args...)
}

// Query runs a query returning rows.
func (s *Store) Query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.DB.QueryContext(ctx, s.Rebind(q), args...)
}

// QueryRow runs a query expected to return one row.
func (s *Store) QueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.DB.QueryRowContext(ctx, s.Rebind(q), args...)
}

// Tx runs fn in a transaction, rolling back on error or panic.
func (s *Store) Tx(ctx context.Context, fn func(*Tx) error) (err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(&Tx{tx: tx, s: s}); err != nil {
		return err
	}
	return tx.Commit()
}

// Tx is a transaction scoped to the same placeholder rules as Store.
type Tx struct {
	tx *sql.Tx
	s  *Store
}

// Exec runs a statement inside the transaction.
func (t *Tx) Exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, t.s.Rebind(q), args...)
}

// QueryRow runs a single-row query inside the transaction.
func (t *Tx) QueryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, t.s.Rebind(q), args...)
}

// Query runs a query inside the transaction.
func (t *Tx) Query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, t.s.Rebind(q), args...)
}

// IsNoRows maps sql.ErrNoRows to ErrNotFound.
func IsNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// IsUnique reports whether err is a unique-constraint violation on either driver.
func IsUnique(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "UNIQUE constraint failed") || strings.Contains(m, "SQLSTATE 23505")
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := s.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return err
		}
		err = s.Tx(ctx, func(tx *Tx) error {
			if _, err := tx.tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().Unix())
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
