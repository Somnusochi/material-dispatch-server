//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMapMaterialSummary(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	username, password := createAdmin(t, pool)
	login(t, client, username, password)
	ctx := context.Background()
	prefix := fmt.Sprintf("统计%d", time.Now().UnixNano())
	location := func(name string, status int) string {
		t.Helper()
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": prefix + name, "loc_type": "warehouse", "address": prefix + name + "地址", "longitude": 119.65, "latitude": 29.1, "status": status}), 201)["id"].(string)
	}
	a, b, empty, inactive, deleted := location("A", 1), location("B", 1), location("_%", 1), location("停用", 0), location("删除", 1)
	spec := func(name, unit string) string {
		t.Helper()
		return requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": prefix + name, "unit": unit}), 201)["id"].(string)
	}
	first, second, third, zero, removed, hidden := spec("配重", "t"), spec("水泥", "t"), spec("钢管", "根"), spec("零库存", "件"), spec("删除", "t"), spec("不可见地点", "t")
	stock := func(loc, material string, normal, damaged, transit int) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty,damaged_qty,in_transit_qty) VALUES($1,$2,$3,$4,$5)`, loc, material, normal, damaged, transit)
		if err != nil {
			t.Fatal(err)
		}
	}
	stock(a, first, 10, 2, 0)
	stock(b, first, 7, 0, 4)
	stock(a, second, 5, 0, 3)
	stock(a, third, 0, 6, 0)
	stock(a, zero, 0, 0, 0)
	stock(a, removed, 999, 999, 999)
	stock(inactive, hidden, 999, 999, 999)
	stock(deleted, hidden, 999, 999, 999)
	if _, err := pool.Exec(ctx, `UPDATE material_dispatch.material_specs SET deleted_at=now() WHERE id=$1`, removed); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE material_dispatch.locations SET deleted_at=now() WHERE id=$1`, deleted); err != nil {
		t.Fatal(err)
	}
	query := func(t *testing.T, material, keyword string) map[string]any {
		t.Helper()
		values := url.Values{"spec_id": {material}, "keyword": {keyword}}
		return requireStatus(t, client.call("GET", "/api/v1/map/summary?"+values.Encode(), nil), 200)
	}
	wantCounts := func(t *testing.T, row map[string]any, locations, normal, damaged, transit float64) {
		t.Helper()
		for key, want := range map[string]float64{"location_count": locations, "normal_spec_count": normal, "damaged_spec_count": damaged, "in_transit_spec_count": transit} {
			if row[key] != want {
				t.Fatalf("%s=%v, want %v; summary=%v", key, row[key], want, row)
			}
		}
	}
	wantLocationCounts := func(t *testing.T, row map[string]any, normal, damaged, transit float64) {
		t.Helper()
		for key, want := range map[string]float64{"normal_spec_count": normal, "damaged_spec_count": damaged, "in_transit_spec_count": transit} {
			if row[key] != want {
				t.Fatalf("location %v: %s=%v, want %v", row["id"], key, row[key], want)
			}
		}
	}
	locationRows := func(t *testing.T) map[string]map[string]any {
		t.Helper()
		rows := map[string]map[string]any{}
		for _, row := range mapRows(t, client, "/api/v1/map/locations") {
			rows[row["id"].(string)] = row
		}
		return rows
	}
	t.Run("location_overview_counts_specs_not_units_or_quantities", func(t *testing.T) {
		rows := locationRows(t)
		wantLocationCounts(t, rows[a], 2, 2, 1)
		wantLocationCounts(t, rows[b], 1, 0, 1)
		wantLocationCounts(t, rows[empty], 0, 0, 0)
		if rows[inactive] != nil || rows[deleted] != nil {
			t.Fatal("inactive or deleted locations must not be returned")
		}
	})
	t.Run("all_materials_deduplicate_across_locations", func(t *testing.T) {
		row := query(t, "", prefix)
		wantCounts(t, row, 3, 2, 2, 2)
		for _, key := range []string{"normal_qty", "damaged_qty", "in_transit_qty", "unit"} {
			if row[key] != nil {
				t.Fatalf("overview must not sum different materials: %s=%v", key, row[key])
			}
		}
	})
	t.Run("single_material_quantities_and_matching_points", func(t *testing.T) {
		row := query(t, first, prefix)
		wantCounts(t, row, 2, 1, 1, 1)
		if row["normal_qty"] != float64(17) || row["damaged_qty"] != float64(2) || row["in_transit_qty"] != float64(4) || row["unit"] != "t" {
			t.Fatal("incorrect single material totals", row)
		}
		rows := mapRows(t, client, "/api/v1/map/locations?spec_id="+first)
		if len(rows) != 2 || rows[0]["id"] != a || rows[1]["id"] != b {
			t.Fatalf("map must only show matching stocked locations: %v", rows)
		}
		wantLocationCounts(t, rows[0], 1, 1, 0)
		wantLocationCounts(t, rows[1], 1, 0, 1)
	})
	t.Run("keyword_and_literal_wildcards", func(t *testing.T) {
		wantCounts(t, query(t, "", prefix+"A地址"), 1, 2, 2, 1)
		wantCounts(t, query(t, "", prefix+"_%"), 1, 0, 0, 0)
		wantCounts(t, query(t, "", prefix+"不存在"), 0, 0, 0, 0)
	})
	t.Run("zero_inventory_keeps_unit", func(t *testing.T) {
		row := query(t, zero, prefix)
		wantCounts(t, row, 0, 0, 0, 0)
		if row["normal_qty"] != float64(0) || row["unit"] != "件" {
			t.Fatal("empty selected material must keep unit", row)
		}
		if rows := mapRows(t, client, "/api/v1/map/locations?spec_id="+zero); len(rows) != 0 {
			t.Fatal("zero stock points must be excluded", rows)
		}
	})
	t.Run("exact_location_selection_distinguishes_duplicate_names", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE material_dispatch.locations SET name=$1 WHERE id=$2`, prefix+"A", b); err != nil {
			t.Fatal(err)
		}
		at := func(location, material string) map[string]any {
			t.Helper()
			values := url.Values{"location_id": {location}, "spec_id": {material}}
			return requireStatus(t, client.call("GET", "/api/v1/map/summary?"+values.Encode(), nil), 200)
		}
		wantCounts(t, at(a, ""), 1, 2, 2, 1)
		wantCounts(t, at(b, ""), 1, 1, 0, 1)
		wantCounts(t, at(empty, ""), 1, 0, 0, 0)
		row := at(a, first)
		wantCounts(t, row, 1, 1, 1, 0)
		if row["normal_qty"] != float64(10) || row["damaged_qty"] != float64(2) || row["in_transit_qty"] != float64(0) || row["unit"] != "t" {
			t.Fatal("selected location quantities must not include its namesake", row)
		}
		row = at(empty, first)
		wantCounts(t, row, 0, 0, 0, 0)
		if row["normal_qty"] != float64(0) || row["unit"] != "t" {
			t.Fatal("empty selected location must retain material unit", row)
		}
		values := url.Values{"location_id": {a}, "keyword": {prefix + "B地址"}}
		wantCounts(t, requireStatus(t, client.call("GET", "/api/v1/map/summary?"+values.Encode(), nil), 200), 0, 0, 0, 0)
	})
	t.Run("location_selection_validates_id_and_visibility", func(t *testing.T) {
		for _, id := range []string{"invalid", "0", "-1", "1 OR 1=1"} {
			requireStatus(t, client.call("GET", "/api/v1/map/summary?location_id="+url.QueryEscape(id), nil), 400)
		}
		for _, id := range []string{inactive, deleted, "9223372036854775807"} {
			requireStatus(t, client.call("GET", "/api/v1/map/summary?location_id="+id, nil), 404)
		}
		requireStatus(t, newClient(t, pool).call("GET", "/api/v1/map/summary?location_id="+a, nil), 401)
	})
	t.Run("more_than_one_hundred_materials", func(t *testing.T) {
		_, err := pool.Exec(ctx, `WITH specs AS (INSERT INTO material_dispatch.material_specs(spec_code,name,category,unit_weight_kg,unit) SELECT $1||n,$1||n,'general',0,'件' FROM generate_series(1,105) n RETURNING id) INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty) SELECT $2,id,1 FROM specs`, prefix, a)
		if err != nil {
			t.Fatal(err)
		}
		wantCounts(t, query(t, "", prefix), 3, 107, 2, 2)
		wantLocationCounts(t, locationRows(t)[a], 107, 2, 1)
	})
	t.Run("authentication_and_input_validation", func(t *testing.T) {
		requireStatus(t, newClient(t, pool).call("GET", "/api/v1/map/summary", nil), 401)
		requireStatus(t, client.call("GET", "/api/v1/map/summary?spec_id=invalid", nil), 400)
		requireStatus(t, client.call("GET", "/api/v1/map/summary?keyword="+strings.Repeat("a", 256), nil), 400)
		requireStatus(t, client.call("GET", "/api/v1/map/summary?spec_id=9223372036854775807", nil), 404)
		requireStatus(t, client.call("GET", "/api/v1/map/summary?spec_id="+removed, nil), 404)
	})
}
