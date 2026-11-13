//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNullDispatchActionHasNoSideEffects(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	location := func(name string) string {
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": name, "loc_type": "warehouse", "longitude": 119.7, "latitude": 29.1, "status": 1}), 201)["id"].(string)
	}
	from, to := location("JSON对象来源"), location("JSON对象目的地")
	spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "JSON对象物料", "unit": "件"}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": 5, "version": 0, "reason": "JSON对象边界测试"}), 200)
	for _, action := range []string{"depart", "cancel"} {
		t.Run(action, func(t *testing.T) {
			id := requireStatus(t, client.call("POST", "/api/v1/dispatches", map[string]any{"from_location_id": from, "to_location_id": to, "spec_id": spec, "dispatch_qty": 1, "priority": 2}), 201)["id"].(string)
			requireStatus(t, client.call("POST", "/api/v1/dispatches/"+id+"/"+action, json.RawMessage(`null`)), 400)
			order := requireStatus(t, client.call("GET", "/api/v1/dispatches/"+id, nil), 200)
			if order["status"] != "pending" {
				t.Fatal("invalid body changed order", order)
			}
			var normal, transit, logs int
			err := pool.QueryRow(context.Background(), `SELECT
			  (SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$3),
			  coalesce((SELECT in_transit_qty FROM material_dispatch.material_inventories WHERE location_id=$2 AND spec_id=$3),0),
			  (SELECT count(*) FROM material_dispatch.operation_logs WHERE module='dispatch' AND target_id=$4 AND action IN ('depart','cancel'))`, from, to, spec, id).Scan(&normal, &transit, &logs)
			if err != nil || normal != 5 || transit != 0 || logs != 0 {
				t.Fatalf("invalid body must not alter inventory/audit: %d %d %d %v", normal, transit, logs, err)
			}
			requireStatus(t, client.call("POST", "/api/v1/dispatches/"+id+"/cancel", map[string]any{}), 200)
		})
	}
}
