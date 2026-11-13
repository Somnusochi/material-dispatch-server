package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSchemaLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated *_test database")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	if !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("refusing destructive migration test outside *_test database")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	provider, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if result, err := provider.Up(ctx); err != nil || len(result) != 0 {
		t.Fatalf("migration not idempotent: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='material_dispatch'`).Scan(&count); err != nil || count != 13 {
		t.Fatalf("expected 13 tables, got %d: %v", count, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`INSERT INTO material_dispatch.sys_users (username,password_hash,real_name) VALUES ('test-user','not-a-login-hash','Test')`,
		`INSERT INTO material_dispatch.locations (name,longitude,latitude) VALUES ('__schema_test_A',119.65,29.08),('__schema_test_B',120.08,29.30)`,
		`INSERT INTO material_dispatch.material_specs (spec_code,name) VALUES ('TEST','test')`,
		`INSERT INTO material_dispatch.material_inventories (location_id,spec_id,normal_qty) SELECT l.id,s.id,10 FROM material_dispatch.locations l CROSS JOIN material_dispatch.material_specs s WHERE l.name IN ('__schema_test_A','__schema_test_B') AND s.spec_code='TEST'`,
		`INSERT INTO material_dispatch.operation_logs (module,action,summary) VALUES ('inventory','create','test')`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	checks := []struct{ sql, code string }{
		{`UPDATE material_dispatch.material_inventories SET normal_qty=-1`, "23514"},
		{`UPDATE material_dispatch.material_inventories SET version=0`, "23514"},
		{`UPDATE material_dispatch.locations SET longitude=181`, "23514"},
		{`UPDATE material_dispatch.sys_users SET role='root'`, "23514"},
		{`INSERT INTO material_dispatch.material_inventories(location_id,spec_id) SELECT location_id,spec_id FROM material_dispatch.material_inventories LIMIT 1`, "23505"},
		{`INSERT INTO material_dispatch.dispatch_orders(order_no,from_location_id,to_location_id,spec_id,dispatch_qty,creator_id) SELECT 'bad',l.id,l.id,s.id,1,u.id FROM material_dispatch.locations l CROSS JOIN material_dispatch.material_specs s CROSS JOIN material_dispatch.sys_users u LIMIT 1`, "23514"},
		{`INSERT INTO material_dispatch.damage_reports(report_no,location_id,spec_id,damaged_qty,description,image_urls,reporter_id) SELECT 'bad',l.id,s.id,1,'bad','{}',u.id FROM material_dispatch.locations l CROSS JOIN material_dispatch.material_specs s CROSS JOIN material_dispatch.sys_users u LIMIT 1`, "23514"},
		{`UPDATE material_dispatch.operation_logs SET summary='tamper'`, "55000"},
		{`DELETE FROM material_dispatch.operation_logs`, "55000"},
	}
	for _, check := range checks {
		if _, err = tx.ExecContext(ctx, "SAVEPOINT constraint_check"); err != nil {
			t.Fatal(err)
		}
		_, err = tx.ExecContext(ctx, check.sql)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != check.code {
			t.Errorf("constraint expected %s, got %v for %s", check.code, err, check.sql)
		}
		if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT constraint_check"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE material_dispatch.material_specs SET name='updated',updated_at='2000-01-01'`); err != nil {
		t.Fatal(err)
	}
	var updated bool
	if err = tx.QueryRowContext(ctx, `SELECT updated_at > '2000-01-02' FROM material_dispatch.material_specs LIMIT 1`).Scan(&updated); err != nil || !updated {
		t.Fatal("updated_at trigger not applied", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.schemata WHERE schema_name='material_dispatch'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rollback failed", err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

// PostgreSQL's default "$user", public search_path changes when a same-named
// schema is created. Migration history must remain stable across connections.
func TestMigrationHistorySurvivesReconnect(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated test database")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil || !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("requires *_test database")
	}
	bootstrap, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Close()
	bootstrap.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = bootstrap.ExecContext(ctx, "SET search_path TO public"); err != nil {
		t.Fatal(err)
	}
	first, err := New(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err = bootstrap.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	status, err := provider.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 5 || status[0].State != "applied" {
		t.Fatalf("migration history lost on reconnect: %+v", status)
	}
}
