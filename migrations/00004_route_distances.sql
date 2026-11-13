-- +goose Up
CREATE TABLE material_dispatch.route_distances (
 from_location_id bigint NOT NULL REFERENCES material_dispatch.locations(id) ON DELETE CASCADE,
 to_location_id bigint NOT NULL REFERENCES material_dispatch.locations(id) ON DELETE CASCADE,
 from_longitude numeric(10,7) NOT NULL,
 from_latitude numeric(10,7) NOT NULL,
 to_longitude numeric(10,7) NOT NULL,
 to_latitude numeric(10,7) NOT NULL,
 distance_m bigint CHECK (distance_m >= 0),
 calculated_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
 last_error varchar(32) NOT NULL DEFAULT '',
 PRIMARY KEY (from_location_id,to_location_id),
 CHECK (from_location_id <> to_location_id),
 CHECK ((distance_m IS NULL) = (calculated_at IS NULL))
);
CREATE INDEX route_distances_due_idx ON material_dispatch.route_distances(next_attempt_at);
CREATE TABLE material_dispatch.route_daily_usage (
 day date PRIMARY KEY,
 requests integer NOT NULL CHECK (requests > 0)
);

-- +goose Down
DROP TABLE material_dispatch.route_daily_usage;
DROP TABLE material_dispatch.route_distances;
