\set ON_ERROR_STOP on
BEGIN;
DO $$
DECLARE expected text;
BEGIN
 FOREACH expected IN ARRAY ARRAY['sys_users','locations','material_specs','material_inventories','dispatch_orders','damage_reports','operation_logs'] LOOP
  IF to_regclass('material_dispatch.' || expected) IS NULL THEN
   RAISE EXCEPTION 'missing expected business table: material_dispatch.%', expected;
  END IF;
 END LOOP;
END $$;
ROLLBACK;
