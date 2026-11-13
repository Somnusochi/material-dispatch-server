package inventory

import "github.com/Somnusochi/material-dispatch-server/internal/platform/business"

// SQL stays in the domain repository; services own validation, state rules and transactions.
const ensureStock business.Statement = `INSERT INTO material_dispatch.material_inventories(location_id,spec_id) VALUES($1,$2) ON CONFLICT(location_id,spec_id) DO NOTHING`

const lockStock business.Statement = `SELECT id,location_id,spec_id,normal_qty,damaged_qty,in_transit_qty,version FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2 FOR UPDATE`

const saveStock business.Statement = `UPDATE material_dispatch.material_inventories SET normal_qty=$1,damaged_qty=$2,in_transit_qty=$3,version=version+1,updated_by=$4 WHERE id=$5`

const lockBalance business.Statement = `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`

const stockExists business.Statement = `SELECT EXISTS(SELECT 1 FROM material_dispatch.material_inventories WHERE location_id=$1 AND spec_id=$2)`

const reservedDamage business.Statement = `SELECT coalesce(sum(damaged_qty),0) FROM material_dispatch.damage_reports WHERE location_id=$1 AND spec_id=$2 AND status='confirmed'`

var listStock = business.Listing{Select: `SELECT i.id::text,i.location_id::text,l.name AS location_name,i.spec_id::text,s.name AS spec_name,s.spec_code,s.unit,s.unit_weight_kg,i.normal_qty,i.damaged_qty,i.in_transit_qty,i.version,i.updated_at`, From: `FROM material_dispatch.material_inventories i JOIN material_dispatch.locations l ON l.id=i.location_id JOIN material_dispatch.material_specs s ON s.id=i.spec_id WHERE ($1='' OR i.location_id::text=$1) AND ($2='' OR i.spec_id::text=$2) AND ($3='' OR l.name ILIKE '%'||$3||'%' OR s.name ILIKE '%'||$3||'%')`}
