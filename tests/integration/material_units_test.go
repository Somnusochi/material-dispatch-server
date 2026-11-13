//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func mapRows(t *testing.T, client *apiClient, path string) []map[string]any {
	t.Helper()
	response := client.call("GET", path, nil)
	requireStatus(t, response, 200)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

func TestMapUnitTotals(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	username, password := createAdmin(t, pool)
	login(t, client, username, password)
	suffix := fmt.Sprint(time.Now().UnixNano())
	location := requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": "单位分组地点", "code": "UNIT-" + suffix, "loc_type": "warehouse", "longitude": 119.65, "latitude": 29.1, "status": 1}), 201)["id"].(string)
	var tons string
	for _, item := range []struct {
		unit     string
		quantity int
	}{{"t", 12}, {"块", 3}, {"t", 8}} {
		spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "单位测试", "spec_code": fmt.Sprintf("%s-%s-%d", suffix, item.unit, item.quantity), "unit": item.unit}), 201)["id"].(string)
		if item.unit == "t" && tons == "" {
			tons = spec
		}
		requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": location, "spec_id": spec, "normal_qty": item.quantity, "damaged_qty": 0, "version": 0, "reason": "单位测试"}), 200)
	}
	check := func(path string, want map[string]float64) {
		t.Helper()
		for _, row := range mapRows(t, client, path) {
			if row["id"] != location {
				continue
			}
			groups, ok := row["stock_by_unit"].([]any)
			if !ok {
				t.Fatal("map must return stock_by_unit", row)
			}
			got := map[string]float64{}
			for _, group := range groups {
				item := group.(map[string]any)
				got[item["unit"].(string)] = item["normal_qty"].(float64)
			}
			if len(got) != len(want) {
				t.Fatalf("units=%v, want %v", got, want)
			}
			for unit, qty := range want {
				if got[unit] != qty {
					t.Fatalf("units=%v, want %v", got, want)
				}
			}
			return
		}
		t.Fatal("map location missing")
	}
	check("/api/v1/map/locations", map[string]float64{"t": 20, "块": 3})
	check("/api/v1/map/locations?spec_id="+tons, map[string]float64{"t": 12})
}

func TestJinhuaDemoSeed(t *testing.T) {
	pool := testPool(t)
	script, err := os.ReadFile("../../scripts/seed-jinhua-demo.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatal(err)
		}
	}
	run()
	var locations, normal, damaged, transit, audits int
	var spec, from, to string
	if err := pool.QueryRow(ctx, `SELECT count(*),sum(i.normal_qty),sum(i.damaged_qty),sum(i.in_transit_qty) FROM material_dispatch.material_inventories i JOIN material_dispatch.material_specs s ON s.id=i.spec_id JOIN material_dispatch.locations l ON l.id=i.location_id WHERE s.spec_code='DEMO-JH-100T-CW' AND s.unit='t' AND s.unit_weight_kg=1000 AND l.code LIKE 'DEMO-JH-100T-%'`).Scan(&locations, &normal, &damaged, &transit); err != nil {
		t.Fatal(err)
	}
	if locations != 15 || normal != 100 || damaged != 0 || transit != 0 {
		t.Fatalf("demo counts: %d, %d, %d, %d", locations, normal, damaged, transit)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM material_dispatch.material_specs WHERE spec_code='DEMO-JH-100T-CW'`).Scan(&spec); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM material_dispatch.locations WHERE code='DEMO-JH-100T-01'`).Scan(&from); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id::text FROM material_dispatch.locations WHERE code='DEMO-JH-100T-02'`).Scan(&to); err != nil {
		t.Fatal(err)
	}
	client := newClient(t, pool)
	username, password := createAdmin(t, pool)
	login(t, client, username, password)
	order := requireStatus(t, client.call("POST", "/api/v1/dispatches", map[string]any{"from_location_id": from, "to_location_id": to, "spec_id": spec, "dispatch_qty": 3, "priority": 2}), 201)
	if order["unit"] != "t" {
		t.Fatal("dispatch lost unit", order)
	}
	orderID := order["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+orderID+"/depart", map[string]any{}), 200)
	if err := pool.QueryRow(ctx, `SELECT sum(normal_qty),sum(in_transit_qty) FROM material_dispatch.material_inventories WHERE spec_id=$1`, spec).Scan(&normal, &transit); err != nil {
		t.Fatal(err)
	}
	if normal != 97 || transit != 3 {
		t.Fatalf("depart must conserve 100t: %d+%d", normal, transit)
	}
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+orderID+"/receive", map[string]any{"normal_qty": 3, "damaged_qty": 0}), 200)
	run()
	if err := pool.QueryRow(ctx, `SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, from, spec).Scan(&normal); err != nil {
		t.Fatal(err)
	}
	if normal != 9 {
		t.Fatalf("repeat seed overwrote dispatched balance: %d", normal)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM material_dispatch.operation_logs WHERE module='demo' AND action='seed' AND target_id='jinhua-counterweight-100t-v1'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("duplicate seed audit: %d", audits)
	}
	total := float64(0)
	for _, row := range mapRows(t, client, "/api/v1/map/locations?spec_id="+spec) {
		for _, v := range row["stock_by_unit"].([]any) {
			group := v.(map[string]any)
			if group["unit"] != "t" {
				t.Fatal("wrong unit", group)
			}
			total += group["normal_qty"].(float64) + group["damaged_qty"].(float64) + group["in_transit_qty"].(float64)
		}
	}
	if total != 100 {
		t.Fatalf("map total after dispatch: %g t", total)
	}
}

func TestDemoSeedDatabaseGuard(t *testing.T) {
	pool := testPool(t)
	script, err := os.ReadFile("../../scripts/seed-jinhua-demo.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, database, address string
		allowed                 bool
	}{
		{"container_test", "material_dispatch_test", "172.18.0.2", true},
		{"local_development", "material_dispatch", "127.0.0.1", true},
		{"remote_development", "material_dispatch", "172.18.0.2", false},
		{"other_database", "production", "127.0.0.1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			// Shadow only the environment probes within this rollback-only test transaction.
			_, err = tx.Exec(ctx, `CREATE SCHEMA demo_guard_test;
CREATE FUNCTION demo_guard_test.current_database() RETURNS name LANGUAGE sql AS 'SELECT current_setting(''test.demo_database'')::name';
CREATE FUNCTION demo_guard_test.inet_server_addr() RETURNS inet LANGUAGE sql AS 'SELECT current_setting(''test.demo_address'')::inet';
SET LOCAL search_path=demo_guard_test,pg_catalog`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `SELECT set_config('test.demo_database',$1,true),set_config('test.demo_address',$2,true)`, tc.database, tc.address); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, string(script))
			if tc.allowed && err != nil {
				t.Fatalf("allowed database was blocked: %v", err)
			}
			if !tc.allowed && (err == nil || !strings.Contains(err.Error(), "Demo seed requires")) {
				t.Fatalf("database guard did not reject target: %v", err)
			}
		})
	}
}
