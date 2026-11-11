package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestMigrateFresh(t *testing.T) {
	db := openTemp(t)
	got, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0001_core", "0002_platform"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("applied %v, want %v", got, want)
	}
	for _, tbl := range []string{"sites", "site_domains", "deployments", "jobs", "sessions",
		"notification_channels", "alert_rule_channels", "event_outbox", "schema_migrations"} {
		if !tableExists(t, db, tbl) {
			t.Errorf("table %s missing", tbl)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	again, err := Migrate(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second run applied %v, want nothing", again)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	db := openTemp(t)
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO sites (id, team_id, slug, name, type, state, created_at, updated_at)
		VALUES ('s1', 'no-such-team', 'x', 'x', 'container', 'draft', 'now', 'now')`)
	if err == nil {
		t.Fatal("expected a foreign key error for a Site with an unknown team")
	}
}

func TestSiteTypeAndStateChecks(t *testing.T) {
	db := openTemp(t)
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO teams (id, name, created_at) VALUES ('t1', 'team', 'now')`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO sites (id, team_id, slug, name, type, state, created_at, updated_at)
		VALUES ('s1', 't1', 'x', 'x', 'ftp', 'draft', 'now', 'now')`)
	if err == nil {
		t.Fatal("expected a CHECK failure for an unknown Site type")
	}
}

func TestDatabaseNewerThanBinary(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES ('9999_future', 'now')`); err != nil {
		t.Fatal(err)
	}
	_, err := Migrate(ctx, db)
	if !errors.Is(err, ErrDatabaseNewer) {
		t.Fatalf("want ErrDatabaseNewer, got %v", err)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	db := openTemp(t)
	fsys := fstest.MapFS{
		"0001_ok.sql":  {Data: []byte(`CREATE TABLE a (id TEXT PRIMARY KEY);`)},
		"0002_bad.sql": {Data: []byte(`CREATE TABLE b (id TEXT PRIMARY KEY); THIS IS NOT SQL;`)},
	}
	got, err := migrate(context.Background(), db, fsys)
	if err == nil {
		t.Fatal("expected the bad migration to fail")
	}
	if !reflect.DeepEqual(got, []string{"0001_ok"}) {
		t.Fatalf("applied %v, want [0001_ok]", got)
	}
	if !tableExists(t, db, "a") {
		t.Error("table a from the good migration should exist")
	}
	if tableExists(t, db, "b") {
		t.Error("table b from the failed migration must be rolled back")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version='0002_bad'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("failed migration must not be recorded")
	}
}

func TestBadMigrationFileName(t *testing.T) {
	db := openTemp(t)
	fsys := fstest.MapFS{"init.sql": {Data: []byte(`SELECT 1;`)}}
	if _, err := migrate(context.Background(), db, fsys); err == nil {
		t.Fatal("expected an error for a badly named migration file")
	}
}
