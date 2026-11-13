//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

func TestDestinationRouteEndpoint(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	prefix := fmt.Sprintf("路线%d", time.Now().UnixNano())
	loc := func(suffix string, status int) string {
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": prefix + suffix, "loc_type": "warehouse", "longitude": 119.65, "latitude": 29.08, "status": status}), 201)["id"].(string)
	}
	from, to := loc("来源", 1), loc("目的地", 1)
	loc("停用", 0)
	endpoint := "/api/v1/locations/" + from + "/destinations"
	result := requireStatus(t, client.call("GET", endpoint+"?keyword="+prefix, nil), 200)
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != to {
		t.Fatalf("wrong destinations: %v", items)
	}
	row := items[0].(map[string]any)
	if row["distance_m"] != nil || row["route_status"] != "unavailable" {
		t.Fatalf("no key must not fabricate a road distance: %v", row)
	}
	requireStatus(t, newClient(t, pool).call("GET", endpoint, nil), 401)
	requireStatus(t, client.call("GET", endpoint+"?page_size=1000", nil), 400)
	requireStatus(t, client.call("GET", "/api/v1/locations/invalid/destinations", nil), 400)
	requireStatus(t, client.call("GET", "/api/v1/locations/9223372036854775807/destinations", nil), 404)
}
