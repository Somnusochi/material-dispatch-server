//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDispatchWithoutTransportInfoAndTerminology(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	ctx := context.Background()
	location := func(name string) string {
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": name, "loc_type": "warehouse", "longitude": 119.7, "latitude": 29.1, "status": 1}), 201)["id"].(string)
	}
	from, to := location("无运输资料来源"), location("无运输资料目的地")
	spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "调度测试物料", "unit": "t"}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": 20, "version": 0, "reason": "调度契约测试"}), 200)
	var created map[string]any
	create := func(qty int) string {
		row := requireStatus(t, client.call("POST", "/api/v1/dispatches", map[string]any{"from_location_id": from, "to_location_id": to, "spec_id": spec, "dispatch_qty": qty, "priority": 2}), 201)
		if row["status"] != "pending" {
			t.Fatal("new order must remain pending", row)
		}
		created = row
		return row["id"].(string)
	}
	id := create(4)
	var normal, transit, audits int
	if err := pool.QueryRow(ctx, `SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, from, spec).Scan(&normal); err != nil || normal != 20 {
		t.Fatal("saving pending order must not change stock", normal, err)
	}
	order := requireStatus(t, client.call("POST", "/api/v1/dispatches/"+id+"/depart", map[string]any{}), 200)
	if order["status"] != "in_transit" {
		t.Fatal("empty JSON must execute dispatch", order)
	}
	t.Run("responses_omit_removed_fields", func(t *testing.T) {
		detail := requireStatus(t, client.call("GET", "/api/v1/dispatches/"+id, nil), 200)
		list := requireStatus(t, client.call("GET", "/api/v1/dispatches?location_id="+from, nil), 200)
		rows := []map[string]any{created, order, detail}
		for _, item := range list["items"].([]any) {
			rows = append(rows, item.(map[string]any))
		}
		for i, row := range rows {
			for _, field := range []string{"driver_name", "driver_phone", "vehicle_plate"} {
				if _, exists := row[field]; exists {
					t.Errorf("response %d still exposes removed field %s", i, field)
				}
			}
		}
	})
	t.Run("new_audits_omit_removed_fields", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM material_dispatch.operation_logs WHERE module='dispatch' AND target_id=$1 AND (before_data ?| ARRAY['driver_name','driver_phone','vehicle_plate'] OR after_data ?| ARRAY['driver_name','driver_phone','vehicle_plate'])`, id).Scan(&count)
		if err != nil || count != 0 {
			t.Fatalf("new audit snapshots must omit removed fields: %d, %v", count, err)
		}
	})
	for action, expected := range map[string]string{"depart": "当前单据无法调度", "cancel": "只能取消待调度的单据"} {
		t.Run(action+"_conflict_message", func(t *testing.T) {
			response := client.call("POST", "/api/v1/dispatches/"+id+"/"+action, map[string]any{})
			requireStatus(t, response, 409)
			var envelope struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != "CONFLICT" || envelope.Error.Message != expected {
				t.Fatalf("got %s: %s; want CONFLICT: %s", envelope.Error.Code, envelope.Error.Message, expected)
			}
		})
	}
	if err := pool.QueryRow(ctx, `SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, from, spec).Scan(&normal); err != nil || normal != 16 {
		t.Fatal("dispatch must deduct once", normal, err)
	}
	if err := pool.QueryRow(ctx, `SELECT in_transit_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, to, spec).Scan(&transit); err != nil || transit != 4 {
		t.Fatal("dispatch must add destination transit once", transit, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM material_dispatch.operation_logs WHERE module='dispatch' AND action='depart' AND target_id=$1`, id).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("dispatch must be audited once", audits, err)
	}
	for _, field := range []string{"driver_name", "driver_phone", "vehicle_plate"} {
		t.Run("reject_removed_"+field, func(t *testing.T) {
			pending := create(1)
			requireStatus(t, client.call("POST", "/api/v1/dispatches/"+pending+"/depart", map[string]any{field: "废弃值"}), 400)
			row := requireStatus(t, client.call("GET", "/api/v1/dispatches/"+pending, nil), 200)
			if row["status"] != "pending" {
				t.Fatal("rejected obsolete fields must not change order state")
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM material_dispatch.operation_logs WHERE module='dispatch' AND action='depart' AND target_id=$1`, pending).Scan(&count); err != nil || count != 0 {
				t.Fatal("rejected request must not create dispatch audit", count, err)
			}
		})
	}
	if err := pool.QueryRow(ctx, `SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, from, spec).Scan(&normal); err != nil || normal != 16 {
		t.Fatal("obsolete fields must not cause stock deductions", normal, err)
	}
}
