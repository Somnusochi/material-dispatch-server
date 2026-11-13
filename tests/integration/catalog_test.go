//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

func TestLocationManagerAndSafeDeactivation(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	user := requireStatus(t, client.call("GET", "/api/v1/auth/me", nil), 200)["user"].(map[string]any)
	manager := user["id"].(string)
	input := map[string]any{"name": "负责人地点", "code": fmt.Sprint(time.Now().UnixNano()), "loc_type": "warehouse", "longitude": 119.6, "latitude": 29.1, "status": 1, "manager_id": manager}
	row := requireStatus(t, client.call("POST", "/api/v1/locations", input), 201)
	if row["manager_id"] != manager || row["manager_name"] != "测试管理员" {
		t.Fatal("manager missing", row)
	}
	contacts := requireStatus(t, client.call("GET", "/api/v1/contacts?keyword="+name, nil), 200)
	if contacts["total"].(float64) != 1 {
		t.Fatal("contact lookup failed", contacts)
	}
	input["status"] = 0
	requireStatus(t, client.call("PUT", "/api/v1/locations/"+row["id"].(string), input), 200)
	input["status"] = 1
	input["manager_id"] = "not-an-id"
	requireStatus(t, client.call("PUT", "/api/v1/locations/"+row["id"].(string), input), 400)
}

func TestListOrdersNumericIDsNewestFirst(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	keyword := "sort_" + fmt.Sprint(time.Now().UnixNano())
	var newest string
	for i := 0; i < 12; i++ {
		row := requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": fmt.Sprintf("%s_%d", keyword, i), "loc_type": "yard", "longitude": 119.6, "latitude": 29.1, "status": 1}), 201)
		newest = row["id"].(string)
	}
	page := requireStatus(t, client.call("GET", "/api/v1/locations?keyword="+keyword+"&page_size=1", nil), 200)
	first := page["items"].([]any)[0].(map[string]any)
	if first["id"] != newest {
		t.Fatalf("numeric newest %s, got %s", newest, first["id"])
	}
}
