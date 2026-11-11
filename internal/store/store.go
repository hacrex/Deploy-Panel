// Package store opens the control plane database and applies migrations.
//
// SQLite is the default. SQL in migrations/ is written to stay portable to
// PostgreSQL; the PostgreSQL path will need placeholder rebinding in the
// runner below (it uses "?").
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"time"

	"github.com/OWNER/deploypanel/migrations"
)

// ErrDatabaseNewer means the database has migrations this binary does not know,
// which happens after a downgrade. The binary refuses to run against it.
var ErrDatabaseNewer = errors.New("database was migrated by a newer version of this program")

var migrationName = regexp.MustCompile(`^\d{4}_[a-z0-9_]+\.sql$`)

// Open opens (and creates, if needed) the SQLite database at path.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// Migrate applies every embedded migration that has not been applied yet and
// returns the versions it applied.
func Migrate(ctx context.Context, db *sql.DB) ([]string, error) {
	return migrate(ctx, db, migrations.FS)
}

func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL)`); err != nil {
		return nil, err
	}

	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	known := map[string]string{} // version -> file
	for _, f := range files {
		if !migrationName.MatchString(f) {
			return nil, fmt.Errorf("bad migration file name %q (want NNNN_name.sql)", f)
		}
		known[f[:len(f)-len(".sql")]] = f
	}

	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return nil, err
		}
		applied[v] = true
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for v := range applied {
		if _, ok := known[v]; !ok {
			return nil, fmt.Errorf("%w: unknown migration %q", ErrDatabaseNewer, v)
		}
	}

	var done []string
	for _, f := range files {
		version := f[:len(f)-len(".sql")]
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(fsys, f)
		if err != nil {
			return done, err
		}
		if err := applyOne(ctx, db, version, string(body)); err != nil {
			return done, fmt.Errorf("migration %s: %w", version, err)
		}
		done = append(done, version)
	}
	return done, nil
}

func applyOne(ctx context.Context, db *sql.DB, version, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}
