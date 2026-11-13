//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInventoryDispatchDamageAndAudit(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	suffix := time.Now().Format("150405000000")
	makeLocation := func(label string) string {
		r := client.call("POST", "/api/v1/locations", map[string]any{"name": label + suffix, "code": label + suffix, "loc_type": "yard", "longitude": 119.65, "latitude": 29.08, "status": 1})
		return requireStatus(t, r, 201)["id"].(string)
	}
	from, to := makeLocation("A"), makeLocation("B")
	spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "5吨配重块", "spec_code": "CW" + suffix, "category": "counterweight", "unit": "块", "unit_weight_kg": 5000}), 201)["id"].(string)
	adjusted := requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": 10, "damaged_qty": 0, "version": 0, "reason": "初始盘点"}), 200)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": -1, "damaged_qty": 0, "version": adjusted["version"], "reason": "无效"}), 400)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": 9, "damaged_qty": 0, "version": 0, "reason": "过期版本"}), 409)
	order := requireStatus(t, client.call("POST", "/api/v1/dispatches", map[string]any{"from_location_id": from, "to_location_id": to, "spec_id": spec, "dispatch_qty": 8, "priority": 2}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+order+"/depart", map[string]any{}), 200)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+order+"/depart", map[string]any{}), 409)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+order+"/receive", map[string]any{"normal_qty": 6, "damaged_qty": 1, "description": "数量不守恒"}), 400)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+order+"/receive", map[string]any{"normal_qty": 7, "damaged_qty": 1, "description": "运输损毁", "image_urls": []string{}}), 200)
	requireStatus(t, client.call("POST", "/api/v1/dispatches/"+order+"/receive", map[string]any{"normal_qty": 7, "damaged_qty": 1}), 409)
	var normal, damaged, transit int
	if err := pool.QueryRow(context.Background(), `SELECT normal_qty,damaged_qty,in_transit_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, to, spec).Scan(&normal, &damaged, &transit); err != nil {
		t.Fatal(err)
	}
	if normal != 7 || damaged != 1 || transit != 0 {
		t.Fatalf("stock after receipt: %d %d %d", normal, damaged, transit)
	}
	var transportReport string
	if err := pool.QueryRow(context.Background(), `SELECT id::text FROM material_dispatch.damage_reports WHERE dispatch_order_id=$1 AND status='confirmed'`, order).Scan(&transportReport); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, client.call("POST", "/api/v1/damage-reports/"+transportReport+"/confirm", map[string]any{}), 409)
	requireStatus(t, client.call("POST", "/api/v1/damage-reports/"+transportReport+"/repair", map[string]any{"remark": "修复"}), 200)
	report := requireStatus(t, client.call("POST", "/api/v1/damage-reports", map[string]any{"location_id": to, "spec_id": spec, "damaged_qty": 2, "description": "盘点发现", "image_urls": []string{}}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/damage-reports/"+report+"/confirm", map[string]any{"remark": "已核实"}), 200)
	requireStatus(t, client.call("POST", "/api/v1/damage-reports/"+report+"/scrap", map[string]any{"remark": "无法修复"}), 200)
	if err := pool.QueryRow(context.Background(), `SELECT normal_qty,damaged_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, to, spec).Scan(&normal, &damaged); err != nil {
		t.Fatal(err)
	}
	if normal != 6 || damaged != 0 {
		t.Fatalf("damage reconciliation: %d %d", normal, damaged)
	}
	requireStatus(t, client.call("GET", "/api/v1/map/locations", nil), 200)
	logs := requireStatus(t, client.call("GET", "/api/v1/operation-logs?module=dispatch", nil), 200)
	if logs["total"].(float64) < 3 {
		t.Fatal("missing transactional audits")
	}
	requireStatus(t, client.call("GET", "/api/v1/inventory?page_size=1000", nil), 400)
}
func TestConcurrentDepartDoesNotOversell(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	suffix := fmt.Sprint(time.Now().UnixNano())
	makeLoc := func(label string) string {
		return requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"name": label, "code": label + suffix, "loc_type": "yard", "longitude": 119.6, "latitude": 29.1, "status": 1}), 201)["id"].(string)
	}
	from, to := makeLoc("A"), makeLoc("B")
	spec := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "测试", "spec_code": suffix, "unit": "块"}), 201)["id"].(string)
	requireStatus(t, client.call("POST", "/api/v1/inventory/adjust", map[string]any{"location_id": from, "spec_id": spec, "normal_qty": 10, "version": 0, "reason": "并发测试"}), 200)
	var ids []string
	for range 2 {
		ids = append(ids, requireStatus(t, client.call("POST", "/api/v1/dispatches", map[string]any{"from_location_id": from, "to_location_id": to, "spec_id": spec, "dispatch_qty": 8, "priority": 2}), 201)["id"].(string))
	}
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- client.call("POST", "/api/v1/dispatches/"+id+"/depart", map[string]any{}).Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("expected one success and one conflict", counts)
	}
	var qty int
	if err := pool.QueryRow(context.Background(), `SELECT normal_qty FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2`, from, spec).Scan(&qty); err != nil || qty != 2 {
		t.Fatal("oversold", qty, err)
	}
}
