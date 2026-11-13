-- +goose Up
-- Back up before applying: these retired field values are not retained here.
SET LOCAL lock_timeout = '5s';
ALTER TABLE material_dispatch.dispatch_orders
 DROP COLUMN driver_name,
 DROP COLUMN driver_phone,
 DROP COLUMN vehicle_plate;

-- +goose Down
-- Restores the old schema only; recover retired values from the pre-upgrade backup.
SET LOCAL lock_timeout = '5s';
ALTER TABLE material_dispatch.dispatch_orders
 ADD COLUMN driver_name varchar(64),
 ADD COLUMN driver_phone varchar(20),
 ADD COLUMN vehicle_plate varchar(32);
