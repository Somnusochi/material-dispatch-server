package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct{ pool *pgxpool.Pool }

const sameCoordinates = `r.from_longitude=f.longitude AND r.from_latitude=f.latitude AND r.to_longitude=d.longitude AND r.to_latitude=d.latitude`

func (r repository) enqueue(ctx context.Context, from int64) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO material_dispatch.route_distances AS r
 (from_location_id,to_location_id,from_longitude,from_latitude,to_longitude,to_latitude)
 SELECT f.id,d.id,f.longitude,f.latitude,d.longitude,d.latitude
 FROM material_dispatch.locations f CROSS JOIN material_dispatch.locations d
 WHERE f.id=$1 AND d.id<>f.id AND f.status=1 AND d.status=1 AND f.deleted_at IS NULL AND d.deleted_at IS NULL
 ON CONFLICT(from_location_id,to_location_id) DO UPDATE SET
 from_longitude=excluded.from_longitude,from_latitude=excluded.from_latitude,
 to_longitude=excluded.to_longitude,to_latitude=excluded.to_latitude,
 distance_m=NULL,calculated_at=NULL,last_error='',next_attempt_at=now()
 WHERE (r.from_longitude,r.from_latitude,r.to_longitude,r.to_latitude) IS DISTINCT FROM
 (excluded.from_longitude,excluded.from_latitude,excluded.to_longitude,excluded.to_latitude)`, from)
	return err
}

func (r repository) list(ctx context.Context, from int64, p business.Page, enabled bool) (DestinationList, error) {
	result := DestinationList{List: business.List{Items: []business.Row{}, Page: p.Page, PageSize: p.Size}}
	const base = `WITH destinations AS (
 SELECT d.id::text,d.name,coalesce(d.address,'') AS address,d.loc_type,
 CASE WHEN ` + sameCoordinates + ` THEN r.distance_m END AS distance_m,
 CASE WHEN ` + sameCoordinates + ` THEN r.calculated_at END AS calculated_at,
 CASE
 WHEN ` + sameCoordinates + ` AND r.distance_m IS NOT NULL THEN
   CASE WHEN r.last_error<>'' OR r.next_attempt_at<=now() THEN 'stale' ELSE 'ready' END
 WHEN NOT $3 THEN 'unavailable'
 WHEN r.last_error<>'' AND ` + sameCoordinates + ` THEN 'unavailable'
 ELSE 'pending' END AS route_status
 FROM material_dispatch.locations f JOIN material_dispatch.locations d ON d.id<>f.id
 LEFT JOIN material_dispatch.route_distances r ON r.from_location_id=f.id AND r.to_location_id=d.id
 WHERE f.id=$1 AND f.status=1 AND f.deleted_at IS NULL AND d.status=1 AND d.deleted_at IS NULL
 AND ($2='' OR strpos(lower(d.name),lower($2))>0 OR strpos(lower(coalesce(d.address,'')),lower($2))>0)
 ) `
	args := []any{from, p.Keyword, enabled}
	if err := r.pool.QueryRow(ctx, base+`SELECT count(*),count(*) FILTER(WHERE route_status='pending') FROM destinations`, args...).Scan(&result.Total, &result.PendingCount); err != nil {
		return result, err
	}
	rows, err := business.Rows(ctx, r.pool, base+`SELECT * FROM destinations ORDER BY distance_m ASC NULLS LAST,id::bigint ASC LIMIT $4 OFFSET $5`, append(args, p.Size, (p.Page-1)*p.Size)...)
	result.Items = rows
	return result, err
}

type routeJob struct {
	fromID, toID int64
	from, to     Point
}

// Only previously requested source/destination pairs are refreshed; no N² sweep.
func dueJobs(ctx context.Context, db business.Queryer, limit int) ([]routeJob, error) {
	_, err := db.Exec(ctx, `UPDATE material_dispatch.route_distances r SET
 from_longitude=f.longitude,from_latitude=f.latitude,to_longitude=d.longitude,to_latitude=d.latitude,
 distance_m=NULL,calculated_at=NULL,last_error='',next_attempt_at=now()
 FROM material_dispatch.locations f,material_dispatch.locations d
 WHERE r.from_location_id=f.id AND r.to_location_id=d.id AND f.status=1 AND d.status=1
 AND f.deleted_at IS NULL AND d.deleted_at IS NULL AND NOT (`+sameCoordinates+`)`)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT r.from_location_id,r.to_location_id,r.from_longitude::float8,r.from_latitude::float8,r.to_longitude::float8,r.to_latitude::float8
 FROM material_dispatch.route_distances r
 JOIN material_dispatch.locations f ON f.id=r.from_location_id AND f.status=1 AND f.deleted_at IS NULL
 JOIN material_dispatch.locations d ON d.id=r.to_location_id AND d.status=1 AND d.deleted_at IS NULL
 WHERE r.next_attempt_at<=now()
 ORDER BY (r.distance_m IS NULL) DESC,r.next_attempt_at,r.from_location_id,r.to_location_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []routeJob{}
	for rows.Next() {
		var j routeJob
		if err = rows.Scan(&j.fromID, &j.toID, &j.from.Longitude, &j.from.Latitude, &j.to.Longitude, &j.to.Latitude); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func saveResult(ctx context.Context, db business.Queryer, j routeJob, meters *int64, next time.Time, code string) error {
	// A coordinate edit during the HTTP call must never publish an obsolete result.
	_, err := db.Exec(ctx, `UPDATE material_dispatch.route_distances r SET
 distance_m=CASE WHEN $7::bigint IS NOT NULL THEN $7 ELSE r.distance_m END,
 calculated_at=CASE WHEN $7::bigint IS NOT NULL THEN now() ELSE r.calculated_at END,
 next_attempt_at=$8,last_error=$9
 FROM material_dispatch.locations f,material_dispatch.locations d
 WHERE r.from_location_id=$1 AND r.to_location_id=$2 AND f.id=$1 AND d.id=$2
 AND r.from_longitude=$3 AND r.from_latitude=$4 AND r.to_longitude=$5 AND r.to_latitude=$6
 AND `+sameCoordinates, j.fromID, j.toID, j.from.Longitude, j.from.Latitude, j.to.Longitude, j.to.Latitude, meters, next, code)
	return err
}

func reserveBudget(ctx context.Context, db business.Queryer, day string, limit int) (bool, error) {
	tag, err := db.Exec(ctx, `INSERT INTO material_dispatch.route_daily_usage AS u(day,requests) VALUES($1,1)
 ON CONFLICT(day) DO UPDATE SET requests=u.requests+1 WHERE u.requests<$2`, day, limit)
	return tag.RowsAffected() > 0, err
}

func postponeDue(ctx context.Context, db business.Queryer, next time.Time, reason string) error {
	_, err := db.Exec(ctx, `UPDATE material_dispatch.route_distances SET next_attempt_at=$1,last_error=$2 WHERE next_attempt_at<=now()`, next, reason)
	return err
}

func activeSource(ctx context.Context, db business.Queryer, id int64) error {
	var found bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM material_dispatch.locations WHERE id=$1 AND status=1 AND deleted_at IS NULL)`, id).Scan(&found); err != nil {
		return err
	}
	if !found {
		return business.NotFound()
	}
	return nil
}

func releaseWorker(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var unlocked bool
	if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock(7364839201)`).Scan(&unlocked); err != nil || !unlocked {
		// Never return a possibly locked session to the pool.
		raw := conn.Hijack()
		_ = raw.Close(ctx)
		return
	}
	conn.Release()
}

func acquireWorker(ctx context.Context, pool *pgxpool.Pool) (*pgxpool.Conn, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(7364839201)`).Scan(&locked); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire route worker lock: %w", err)
	}
	if !locked {
		conn.Release()
		return nil, nil
	}
	return conn, nil
}
