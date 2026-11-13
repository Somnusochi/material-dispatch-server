//go:build integration

package routing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testProvider struct {
	calls atomic.Int32
	fail  bool
}

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (p *blockingProvider) Distance(ctx context.Context, _, _ Point) (int64, error) {
	p.calls.Add(1)
	close(p.started)
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-p.release:
		return 1000, nil
	}
}

func TestWorkerLockAndCoordinateEditDuringCalculation(t *testing.T) {
	ctx := context.Background()
	pool := routePool(t)
	if _, err := pool.Exec(ctx, `TRUNCATE material_dispatch.route_distances,material_dispatch.route_daily_usage`); err != nil {
		t.Fatal(err)
	}
	var from, to int64
	for i, target := range []*int64{&from, &to} {
		if err := pool.QueryRow(ctx, `INSERT INTO material_dispatch.locations(name,longitude,latitude) VALUES($1,119.6,29.1) RETURNING id`, fmt.Sprintf("并发路线%d-%d", time.Now().UnixNano(), i)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO material_dispatch.route_distances(from_location_id,to_location_id,from_longitude,from_latitude,to_longitude,to_latitude) VALUES($1,$2,119.6,29.1,119.6,29.1)`, from, to); err != nil {
		t.Fatal(err)
	}
	p := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	s := NewService(pool, p, Options{RefreshHour: 2, DailyLimit: 10})
	s.interval = 0
	done := make(chan error, 1)
	go func() { _, err := s.RefreshBatch(ctx, 1); done <- err }()
	<-p.started
	// A second instance must not issue the same external call.
	other := NewService(pool, p, Options{RefreshHour: 2, DailyLimit: 10})
	if count, err := other.RefreshBatch(ctx, 1); err != nil || count != 0 {
		t.Fatal("worker lock failed", count, err)
	}
	_, editErr := pool.Exec(ctx, `UPDATE material_dispatch.locations SET longitude=120 WHERE id=$1`, to)
	close(p.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if editErr != nil {
		t.Fatal(editErr)
	}
	var distance *int64
	if err := pool.QueryRow(ctx, `SELECT distance_m FROM material_dispatch.route_distances WHERE from_location_id=$1 AND to_location_id=$2`, from, to).Scan(&distance); err != nil {
		t.Fatal(err)
	}
	if distance != nil || p.calls.Load() != 1 {
		t.Fatal("obsolete coordinate result published or duplicated")
	}
	// The lock was released and a new calculation can now refresh the new coordinates.
	fresh := &testProvider{}
	s = NewService(pool, fresh, Options{RefreshHour: 2, DailyLimit: 10})
	s.interval = 0
	if n, err := s.RefreshBatch(ctx, 1); err != nil || n != 1 {
		t.Fatal("worker did not resume", n, err)
	}
	var next time.Time
	if err := pool.QueryRow(ctx, `SELECT distance_m,next_attempt_at FROM material_dispatch.route_distances WHERE from_location_id=$1 AND to_location_id=$2`, from, to).Scan(&distance, &next); err != nil {
		t.Fatal(err)
	}
	if distance == nil || next.In(shanghai).Hour() != 2 || !next.After(time.Now()) {
		t.Fatal("daily schedule missing", next, distance)
	}
	// A past schedule catches up on the next worker pass, without an HTTP read.
	if _, err := pool.Exec(ctx, `UPDATE material_dispatch.route_distances SET next_attempt_at=now()-interval '2 days'`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.RefreshBatch(ctx, 1); err != nil || n != 1 || fresh.calls.Load() != 2 {
		t.Fatal("missed schedule not caught up", n, err)
	}
}

func (p *testProvider) Distance(_ context.Context, from, to Point) (int64, error) {
	p.calls.Add(1)
	if p.fail {
		return 0, errors.New("provider unavailable")
	}
	if to.Longitude == 119.7 {
		return 800, nil
	}
	return 25000, nil
}

func routePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires *_test database")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRouteCacheAndWorker(t *testing.T) {
	ctx := context.Background()
	pool := routePool(t)
	// Isolated fixtures for this package; full integration runs packages sequentially.
	if _, err := pool.Exec(ctx, `TRUNCATE material_dispatch.route_distances,material_dispatch.route_daily_usage`); err != nil {
		t.Fatal(err)
	}
	prefix := time.Now().Format("150405.000000000")
	loc := func(suffix string, lon float64) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO material_dispatch.locations(name,longitude,latitude) VALUES($1,$2,29.1) RETURNING id::text`, prefix+suffix, lon).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	from, far, near := loc("来源", 119.6), loc("远处", 119.9), loc("近处", 119.7)
	provider := &testProvider{}
	s := NewService(pool, provider, Options{RefreshHour: 2, DailyLimit: 1000})
	s.interval = 0
	page := business.Page{Page: 1, Size: 1, Keyword: prefix}
	result, err := s.Destinations(ctx, from, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.PendingCount != 2 {
		t.Fatalf("pending list: %+v", result)
	}
	// Enqueue only the fixtures, keeping this test independent from prior suites.
	if _, err = pool.Exec(ctx, `DELETE FROM material_dispatch.route_distances WHERE from_location_id::text<>$1 OR to_location_id::text NOT IN ($2,$3)`, from, near, far); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RefreshBatch(ctx, 10); err != nil {
		t.Fatal(err)
	}
	// Reads must use cached values without calling the provider.
	result, err = s.Destinations(ctx, from, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0]["id"] != near || result.Items[0]["distance_m"] != float64(800) {
		t.Fatalf("not globally sorted before pagination: %+v", result)
	}
	page.Page = 2
	result, err = s.Destinations(ctx, from, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0]["id"] != far || provider.calls.Load() != 2 {
		t.Fatal("cache/pagination wrong", result, provider.calls.Load())
	}
	// Moving the destination invalidates the old distance immediately, before any worker runs.
	if _, err = pool.Exec(ctx, `UPDATE material_dispatch.locations SET longitude=119.8 WHERE id=$1`, near); err != nil {
		t.Fatal(err)
	}
	page.Page = 1
	page.Size = 100
	result, err = s.Destinations(ctx, from, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[1]["id"] != near || result.Items[1]["distance_m"] != nil || result.Items[1]["route_status"] != "pending" {
		t.Fatal("coordinates did not invalidate cache", result)
	}
	// A reverse route never borrows the outbound distance.
	reversed, err := s.Destinations(ctx, far, page)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range reversed.Items {
		if row["id"] == from && row["distance_m"] != nil {
			t.Fatal("reverse route reused", row)
		}
	}
	// Provider failures preserve a same-coordinate previous success, back off, and retain sorting.
	if _, err = pool.Exec(ctx, `DELETE FROM material_dispatch.route_distances WHERE from_location_id::text<>$1 OR to_location_id::text<>$2`, from, far); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE material_dispatch.route_distances SET next_attempt_at=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	provider.fail = true
	if _, err = s.RefreshBatch(ctx, 10); err != nil {
		t.Fatal(err)
	}
	result, err = s.Destinations(ctx, from, page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0]["id"] != far || result.Items[0]["distance_m"] != float64(25000) || result.Items[0]["route_status"] != "stale" {
		t.Fatal("failed refresh lost old distance", result)
	}
	var backedOff bool
	if err = pool.QueryRow(ctx, `SELECT next_attempt_at>now()+interval '10 minutes' FROM material_dispatch.route_distances WHERE from_location_id::text=$1 AND to_location_id::text=$2`, from, far).Scan(&backedOff); err != nil || !backedOff {
		t.Fatal("no retry backoff", err)
	}
	// The persistent daily budget is shared with other workers and survives service restarts.
	if _, err = pool.Exec(ctx, `UPDATE material_dispatch.route_distances SET next_attempt_at=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	s.dailyLimit = 1
	before := provider.calls.Load()
	if _, err = s.RefreshBatch(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if provider.calls.Load() != before {
		t.Fatal("daily budget ignored")
	}
}

type deniedProvider struct{}

func (deniedProvider) Distance(context.Context, Point, Point) (int64, error) {
	return 0, errProviderDenied
}

func TestProviderDenialPausesFailedPairToo(t *testing.T) {
	ctx := context.Background()
	pool := routePool(t)
	if _, err := pool.Exec(ctx, `TRUNCATE material_dispatch.route_distances,material_dispatch.route_daily_usage`); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO material_dispatch.route_distances(from_location_id,to_location_id,from_longitude,from_latitude,to_longitude,to_latitude)
 SELECT f.id,d.id,f.longitude,f.latitude,d.longitude,d.latitude FROM material_dispatch.locations f,material_dispatch.locations d
 WHERE f.id<>d.id AND f.status=1 AND d.status=1 AND f.deleted_at IS NULL AND d.deleted_at IS NULL LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(pool, deniedProvider{}, Options{RefreshHour: 2, DailyLimit: 10})
	s.interval = 0
	if n, err := s.RefreshBatch(ctx, 2); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	var allPaused bool
	if err := pool.QueryRow(ctx, `SELECT bool_and(next_attempt_at>now()+interval '50 minutes' AND last_error='provider_denied') FROM material_dispatch.route_distances`).Scan(&allPaused); err != nil || !allPaused {
		t.Fatal("failed pair and remaining pairs must both pause for an hour", err)
	}
}
