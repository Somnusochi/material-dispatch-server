package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDispatchTransportColumnsMigration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated test database")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil || !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("requires *_test database")
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
	if _, err = provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO material_dispatch.sys_users(username,password_hash,real_name) VALUES ('migration-test','not-a-login-hash','迁移测试')`,
		`INSERT INTO material_dispatch.locations(name,longitude,latitude) VALUES ('迁移来源',119.7,29.1),('迁移目的地',119.8,29.2)`,
		`INSERT INTO material_dispatch.material_specs(spec_code,name,unit) VALUES ('MIGRATION-TRANSPORT','迁移测试物料','t')`,
		`INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty,in_transit_qty) VALUES (1,1,6,0),(2,1,0,4)`,
		`INSERT INTO material_dispatch.dispatch_orders(order_no,from_location_id,to_location_id,spec_id,dispatch_qty,status,creator_id,dispatcher_id,departed_at,driver_name,driver_phone,vehicle_plate,remark) VALUES ('MIGRATION-TRANSPORT',1,2,1,4,'in_transit',1,1,now(),'历史司机','历史电话','历史车牌','迁移保留备注')`,
		`INSERT INTO material_dispatch.operation_logs(module,action,target_id,summary,after_data) VALUES ('dispatch','depart','1','历史审计',jsonb_build_object('id','1','driver_name','历史司机','dispatch_qty',4))`,
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	// Compare all surviving order fields, stock rows and append-only audit data.
	const snapshot = `SELECT jsonb_build_object(
		'order',(SELECT to_jsonb(d)-ARRAY['driver_name','driver_phone','vehicle_plate'] FROM material_dispatch.dispatch_orders d WHERE id=1),
		'stock',(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM material_dispatch.material_inventories i),
		'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM material_dispatch.operation_logs a))::text`
	var before, after string
	if err = db.QueryRowContext(ctx, snapshot).Scan(&before); err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func() {
		t.Helper()
		if err := db.QueryRowContext(ctx, snapshot).Scan(&after); err != nil || after != before {
			t.Fatalf("migration changed surviving order/stock/audit data: %v", err)
		}
	}
	assertRemoved := func() {
		t.Helper()
		var count int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='material_dispatch' AND table_name='dispatch_orders' AND column_name IN ('driver_name','driver_phone','vehicle_plate')`).Scan(&count)
		if err != nil || count != 0 {
			t.Fatalf("expected transport columns removed, got %d: %v", count, err)
		}
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	assertRemoved()
	assertUnchanged()
	if result, err := provider.Up(ctx); err != nil || len(result) != 0 {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	if _, err = provider.DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	var columns int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='material_dispatch' AND table_name='dispatch_orders' AND data_type='character varying' AND is_nullable='YES' AND ((column_name='driver_name' AND character_maximum_length=64) OR (column_name='driver_phone' AND character_maximum_length=20) OR (column_name='vehicle_plate' AND character_maximum_length=32))`).Scan(&columns)
	if err != nil || columns != 3 {
		t.Fatalf("rollback must restore the three nullable column definitions: %d, %v", columns, err)
	}
	var empty bool
	if err = db.QueryRowContext(ctx, `SELECT driver_name IS NULL AND driver_phone IS NULL AND vehicle_plate IS NULL FROM material_dispatch.dispatch_orders WHERE id=1`).Scan(&empty); err != nil || !empty {
		t.Fatalf("rollback restores structure only; values require a backup: %v", err)
	}
	assertUnchanged()
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	assertRemoved()
	assertUnchanged()
}
