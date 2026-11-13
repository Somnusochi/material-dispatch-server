//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

func TestDamageReportWithoutLevel(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	loc := requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": "损毁简化测试仓库", "loc_type": "warehouse", "longitude": 119.65, "latitude": 29.08, "status": 1}), 201)["id"].(string)
	spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "测试钢管", "unit": "根"}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": loc, "spec_id": spec, "normal_qty": 10, "damaged_qty": 0, "version": 0, "reason": "独立测试初始库存"}), 200)
	photo := specPhoto(t, client)
	input := func() map[string]any {
		return map[string]any{"location_id": loc, "spec_id": spec, "damaged_qty": 2, "description": "10根中2根弯曲", "image_urls": []string{photo}}
	}
	stock := func(t *testing.T, wantNormal, wantDamaged int) {
		t.Helper()
		var normal, damaged int
		if err := pool.QueryRow(context.Background(), `SELECT normal_qty,damaged_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, loc, spec).Scan(&normal, &damaged); err != nil {
			t.Fatal(err)
		}
		if normal != wantNormal || damaged != wantDamaged {
			t.Fatalf("stock = %d/%d, want %d/%d", normal, damaged, wantNormal, wantDamaged)
		}
	}
	for _, variant := range []string{"omitted", "null", "empty"} {
		t.Run(variant, func(t *testing.T) {
			body := input()
			if variant == "null" {
				body["damage_level"] = nil
			} else if variant == "empty" {
				body["damage_level"] = ""
			}
			report := requireStatus(t, client.call("POST", "/api/v1/damage-reports", body), 201)
			if report["status"] != "pending" || report["damaged_qty"] != float64(2) || report["unit"] != "根" || report["description"] != body["description"] {
				t.Fatal("incorrect damage report", report)
			}
			images, ok := report["image_urls"].([]any)
			if !ok || len(images) != 1 || images[0] != photo {
				t.Fatal("missing damage photo", report)
			}
			stock(t, 10, 0)
			id := report["id"].(string)
			var creates int
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM material_dispatch.operation_logs WHERE module='damage' AND action='create' AND target_id=$1 AND after_data->>'status'='pending' AND after_data->>'damaged_qty'='2'`, id).Scan(&creates); err != nil || creates != 1 {
				t.Fatal("missing creation audit", creates, err)
			}
			path := "/api/v1/damage-reports/" + id
			requireStatus(t, client.call("POST", path+"/confirm", map[string]any{"remark": "按损毁数量核实"}), 200)
			stock(t, 8, 2)
			requireStatus(t, client.call("POST", path+"/confirm", map[string]any{"remark": "重复确认"}), 409)
			stock(t, 8, 2)
			requireStatus(t, client.call("POST", path+"/repair", map[string]any{"remark": "2根已修复"}), 200)
			stock(t, 10, 0)
		})
	}
	t.Run("legacy_parameters_remain_compatible", func(t *testing.T) {
		for _, level := range []string{"minor", "moderate", "scrapped"} {
			body := input()
			body["damage_level"] = level
			report := requireStatus(t, client.call("POST", "/api/v1/damage-reports", body), 201)
			read := requireStatus(t, client.call("GET", "/api/v1/damage-reports/"+report["id"].(string), nil), 200)
			if read["damage_level"] != level || read["status"] != "pending" {
				t.Fatal("legacy input changed", read)
			}
			stock(t, 10, 0)
		}
	})
	t.Run("quantity_description_and_photo_validation_remain", func(t *testing.T) {
		for _, tc := range []struct {
			field   string
			value   any
			message string
		}{
			{"damaged_qty", 0, "损毁数量"},
			{"damaged_qty", -1, "损毁数量"},
			{"damaged_qty", 100000001, "损毁数量"},
			{"description", " ", "损毁说明"},
			{"description", strings.Repeat("坏", 2001), "损毁说明"},
			{"image_urls", []string{"https://example.com/untrusted.png"}, "图片"},
			{"damage_level", "invalid", "损毁等级"},
		} {
			body := input()
			body[tc.field] = tc.value
			response := client.call("POST", "/api/v1/damage-reports", body)
			requireStatus(t, response, 400)
			if !strings.Contains(response.Body.String(), tc.message) {
				t.Errorf("wrong validation for %s: %s", tc.field, response.Body)
			}
		}
		stock(t, 10, 0)
	})
	t.Run("authentication_required", func(t *testing.T) {
		requireStatus(t, newClient(t, pool).call("POST", "/api/v1/damage-reports", input()), 401)
	})
}
