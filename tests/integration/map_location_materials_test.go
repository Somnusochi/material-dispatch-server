//go:build integration

package integration

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestMapLocationMaterials(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	username, password := createAdmin(t, pool)
	login(t, client, username, password)
	ctx := context.Background()
	prefix := fmt.Sprintf("地点物料%d", time.Now().UnixNano())
	location := func(name string, status int) string {
		t.Helper()
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": prefix + name, "loc_type": "warehouse", "longitude": 119.65, "latitude": 29.1, "status": status}), 201)["id"].(string)
	}
	loc, empty, inactive := location("多物料", 1), location("空地点", 1), location("停用", 0)
	material := func(name, unit string, normal, damaged, transit int, deleted bool) string {
		t.Helper()
		spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": name, "unit": unit}), 201)["id"].(string)
		if _, err := pool.Exec(ctx, `INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty,damaged_qty,in_transit_qty) VALUES($1,$2,$3,$4,$5)`, loc, spec, normal, damaged, transit); err != nil {
			t.Fatal(err)
		}
		if deleted {
			if _, err := pool.Exec(ctx, `UPDATE material_dispatch.material_specs SET deleted_at=now() WHERE id=$1`, spec); err != nil {
				t.Fatal(err)
			}
		}
		return spec
	}
	first := material("配重块", "t", 10, 2, 0, false)
	material("水泥", "t", 5, 0, 3, false)
	material("钢管", "根", 0, 6, 0, false)
	material("零库存", "件", 0, 0, 0, false)
	material("已删除", "t", 999, 999, 999, true)
	point := func(t *testing.T, id, spec string) map[string]any {
		t.Helper()
		for _, row := range mapRows(t, client, "/api/v1/map/locations?spec_id="+spec) {
			if row["id"] == id {
				return row
			}
		}
		t.Fatalf("location %s is missing", id)
		return nil
	}
	items := func(t *testing.T, row map[string]any, key string, count int) []any {
		t.Helper()
		rows, ok := row[key].([]any)
		if !ok || len(rows) != count {
			t.Fatalf("%s=%v, want %d material rows", key, row[key], count)
		}
		return rows
	}
	t.Run("preview_separates_materials_and_partial_damage", func(t *testing.T) {
		row := point(t, loc, "")
		rows := items(t, row, "materials", 3)
		if row["material_count"] != float64(3) {
			t.Fatal("wrong full material count", row)
		}
		for index, want := range []struct {
			name, unit               string
			normal, damaged, transit float64
		}{{"钢管", "根", 0, 6, 0}, {"水泥", "t", 5, 0, 3}, {"配重块", "t", 10, 2, 0}} {
			stock := rows[index].(map[string]any)
			if stock["spec_name"] != want.name || stock["unit"] != want.unit || stock["normal_qty"] != want.normal || stock["damaged_qty"] != want.damaged || stock["in_transit_qty"] != want.transit {
				t.Fatalf("wrong per-material stock: %v", stock)
			}
			if _, ok := stock["spec_id"].(string); !ok {
				t.Fatal("spec ID must remain a string", stock)
			}
		}
		emptyRow := point(t, empty, "")
		items(t, emptyRow, "materials", 0)
		if emptyRow["material_count"] != float64(0) {
			t.Fatal("empty location count must be zero", emptyRow)
		}
		selected := point(t, loc, first)
		selectedItems := items(t, selected, "materials", 1)
		if selected["material_count"] != float64(1) || selectedItems[0].(map[string]any)["normal_qty"] != float64(10) {
			t.Fatal("selected spec must not include other materials", selected)
		}
	})
	t.Run("pagination_matches_preview_and_covers_more_than_100_materials", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `WITH specs AS (INSERT INTO material_dispatch.material_specs(spec_code,name,category,unit_weight_kg,unit) SELECT $1||n,$1||n,'general',0,'件' FROM generate_series(1,105) n RETURNING id) INSERT INTO material_dispatch.material_inventories(location_id,spec_id,normal_qty) SELECT $2,id,1 FROM specs`, prefix, loc); err != nil {
			t.Fatal(err)
		}
		row := point(t, loc, "")
		preview := items(t, row, "materials", 3)
		if row["material_count"] != float64(108) {
			t.Fatal("preview count must include all pages", row)
		}
		seen := map[string]bool{}
		for page := 1; page <= 36; page++ {
			result := requireStatus(t, client.call("GET", fmt.Sprintf("/api/v1/map/locations/%s/materials?page=%d&page_size=3", loc, page), nil), 200)
			rows := items(t, result, "items", 3)
			if result["total"] != float64(108) || result["page"] != float64(page) || result["page_size"] != float64(3) {
				t.Fatal("wrong pagination metadata", result)
			}
			if page == 1 && !reflect.DeepEqual(rows, preview) {
				t.Fatal("preview and first page must be identical", rows, preview)
			}
			for _, item := range rows {
				id := item.(map[string]any)["spec_id"].(string)
				if seen[id] {
					t.Fatal("duplicate material between pages", id)
				}
				seen[id] = true
			}
		}
		if len(seen) != 108 {
			t.Fatal("pagination lost materials", len(seen))
		}
	})
	t.Run("authenticated_read_and_validation", func(t *testing.T) {
		path := "/api/v1/map/locations/" + loc + "/materials"
		requireStatus(t, newClient(t, pool).call("GET", path, nil), 401)
		requireStatus(t, client.call("GET", path+"?page=0", nil), 400)
		requireStatus(t, client.call("GET", path+"?page_size=101", nil), 400)
		requireStatus(t, client.call("GET", "/api/v1/map/locations/invalid/materials", nil), 400)
		requireStatus(t, client.call("GET", "/api/v1/map/locations/"+inactive+"/materials", nil), 404)
		requireStatus(t, client.call("GET", "/api/v1/map/locations/9223372036854775807/materials", nil), 404)
		result := requireStatus(t, client.call("GET", "/api/v1/map/locations/"+empty+"/materials", nil), 200)
		items(t, result, "items", 0)
	})
}
