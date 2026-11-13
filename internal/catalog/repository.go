package catalog

import "github.com/Somnusochi/material-dispatch-server/internal/platform/business"

// SQL stays in the domain repository; services own validation, state rules and transactions.
const locationSelect = `SELECT l.id::text,l.name,l.code,l.loc_type,coalesce(l.address,'') AS address,l.longitude,l.latitude,l.coordinate_system,l.manager_id::text,coalesce((SELECT u.real_name FROM material_dispatch.sys_users u WHERE u.id=l.manager_id),'') AS manager_name,coalesce(l.contact_phone,'') AS contact_phone,l.status,coalesce(l.remark,'') AS remark,l.created_at,l.updated_at`

const specSelect = `SELECT s.id::text,s.spec_code,s.name,s.category,s.unit_weight_kg,coalesce(s.dimensions,'') AS dimensions,s.unit,coalesce(s.image_url,'') AS image_url,coalesce(s.remark,'') AS remark,s.created_at,s.updated_at`

var listLocations = business.Listing{Select: locationSelect, From: `FROM material_dispatch.locations l WHERE l.deleted_at IS NULL AND ($1='' OR l.name ILIKE '%'||$1||'%' OR l.code ILIKE '%'||$1||'%' OR l.address ILIKE '%'||$1||'%') AND ($2='' OR l.status::text=$2)`}

const lockLocation business.Statement = locationSelect + ` FROM material_dispatch.locations l WHERE l.id=$1 AND l.deleted_at IS NULL FOR UPDATE`

const locationInUse business.Statement = `SELECT EXISTS(SELECT 1 FROM material_dispatch.material_inventories WHERE location_id=$1 AND normal_qty+damaged_qty+in_transit_qty>0) OR EXISTS(SELECT 1 FROM material_dispatch.dispatch_orders WHERE (from_location_id=$1 OR to_location_id=$1) AND status IN ('pending','in_transit'))`

const updateLocation business.Statement = `UPDATE material_dispatch.locations SET name=$1,code=nullif($2,''),loc_type=$3,address=$4,longitude=$5,latitude=$6,contact_phone=$7,status=$8,remark=$9,manager_id=$10 WHERE id=$11`

const insertLocation business.Statement = `INSERT INTO material_dispatch.locations(name,code,loc_type,address,longitude,latitude,contact_phone,status,remark,manager_id) VALUES($1,nullif($2,''),$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`

const getLocation business.Statement = locationSelect + ` FROM material_dispatch.locations l WHERE l.id=$1`

var listSpecs = business.Listing{Select: specSelect, From: `FROM material_dispatch.material_specs s WHERE s.deleted_at IS NULL AND ($1='' OR s.name ILIKE '%'||$1||'%' OR s.spec_code ILIKE '%'||$1||'%')`}

const lockSpec business.Statement = specSelect + ` FROM material_dispatch.material_specs s WHERE s.id=$1 AND s.deleted_at IS NULL FOR UPDATE`

const updateSpec business.Statement = `UPDATE material_dispatch.material_specs SET spec_code=coalesce($1,spec_code),name=$2,category=coalesce($3,category),unit_weight_kg=coalesce($4,unit_weight_kg),dimensions=coalesce($5,dimensions),unit=$6,image_url=coalesce($7,image_url),remark=coalesce($8,remark) WHERE id=$9`

const insertSpec business.Statement = `INSERT INTO material_dispatch.material_specs(spec_code,name,category,unit_weight_kg,dimensions,unit,image_url,remark) VALUES($1,$2,coalesce($3,'general'),coalesce($4::numeric,0),coalesce($5,''),$6,coalesce($7,''),coalesce($8,'')) RETURNING id`

const getSpec business.Statement = specSelect + ` FROM material_dispatch.material_specs s WHERE s.id=$1`

const activeLocation business.Statement = `SELECT status FROM material_dispatch.locations WHERE id=$1 AND deleted_at IS NULL FOR SHARE`

const activeSpec business.Statement = `SELECT id FROM material_dispatch.material_specs WHERE id=$1 AND deleted_at IS NULL FOR SHARE`

const uploadExists business.Statement = `SELECT EXISTS(SELECT 1 FROM material_dispatch.uploads WHERE id=$1)`

const mapStockFrom = `FROM material_dispatch.material_inventories i
 JOIN material_dispatch.locations l ON l.id=i.location_id
 JOIN material_dispatch.material_specs s ON s.id=i.spec_id
 WHERE l.status=1 AND l.deleted_at IS NULL AND s.deleted_at IS NULL`

const positiveMapStock = `(i.normal_qty>0 OR i.damaged_qty>0 OR i.in_transit_qty>0)`

const mapMaterialSelect = `SELECT i.id::text,i.spec_id::text,s.name AS spec_name,s.unit,i.normal_qty,i.damaged_qty,i.in_transit_qty`

var listMapMaterials = business.Listing{Select: mapMaterialSelect, From: mapStockFrom + ` AND i.location_id=$1 AND ` + positiveMapStock + ` AND ($2='' OR strpos(s.name,$2)>0) AND ($3='' OR i.spec_id::text=$3)`}

const activeMapLocation business.Statement = `SELECT id::text FROM material_dispatch.locations WHERE id=$1 AND status=1 AND deleted_at IS NULL`

const mapLocations business.Statement = `WITH visible_stock AS (
 SELECT i.id,i.location_id,i.spec_id,s.name AS spec_name,s.unit,i.normal_qty,i.damaged_qty,i.in_transit_qty
 ` + mapStockFrom + ` AND ($1='' OR i.spec_id::text=$1)
), unit_stock AS (
 SELECT i.location_id,i.unit,sum(i.normal_qty) AS normal_qty,sum(i.damaged_qty) AS damaged_qty,sum(i.in_transit_qty) AS in_transit_qty,
 count(DISTINCT i.spec_id) FILTER (WHERE i.normal_qty>0) AS normal_spec_count,
 count(DISTINCT i.spec_id) FILTER (WHERE i.damaged_qty>0) AS damaged_spec_count,
 count(DISTINCT i.spec_id) FILTER (WHERE i.in_transit_qty>0) AS in_transit_spec_count
 FROM visible_stock i GROUP BY i.location_id,i.unit
), location_stock AS (
 SELECT location_id,sum(normal_qty) AS normal_qty,sum(damaged_qty) AS damaged_qty,sum(in_transit_qty) AS in_transit_qty,
 sum(normal_spec_count)::bigint AS normal_spec_count,sum(damaged_spec_count)::bigint AS damaged_spec_count,sum(in_transit_spec_count)::bigint AS in_transit_spec_count,
 jsonb_agg(jsonb_build_object('unit',unit,'normal_qty',normal_qty,'damaged_qty',damaged_qty,'in_transit_qty',in_transit_qty) ORDER BY unit) AS stock_by_unit
 FROM unit_stock GROUP BY location_id
), ranked_materials AS (
 SELECT i.*,row_number() OVER (PARTITION BY i.location_id ORDER BY i.id DESC) AS position
 FROM visible_stock i WHERE ` + positiveMapStock + `
), material_lists AS (
 SELECT location_id,count(*) AS material_count,
 jsonb_agg(jsonb_build_object('id',id::text,'spec_id',spec_id::text,'spec_name',spec_name,'unit',unit,'normal_qty',normal_qty,'damaged_qty',damaged_qty,'in_transit_qty',in_transit_qty) ORDER BY id DESC) FILTER (WHERE position<=3) AS materials
 FROM ranked_materials GROUP BY location_id
) ` + locationSelect + `,coalesce(i.normal_qty,0) AS normal_qty,coalesce(i.damaged_qty,0) AS damaged_qty,coalesce(i.in_transit_qty,0) AS in_transit_qty,coalesce(i.stock_by_unit,'[]'::jsonb) AS stock_by_unit,
 coalesce(i.normal_spec_count,0) AS normal_spec_count,coalesce(i.damaged_spec_count,0) AS damaged_spec_count,coalesce(i.in_transit_spec_count,0) AS in_transit_spec_count,
 coalesce(m.material_count,0) AS material_count,coalesce(m.materials,'[]'::jsonb) AS materials
 FROM material_dispatch.locations l LEFT JOIN location_stock i ON i.location_id=l.id
 LEFT JOIN material_lists m ON m.location_id=l.id
 WHERE l.deleted_at IS NULL AND l.status=1
 AND ($1='' OR i.normal_qty>0 OR i.damaged_qty>0 OR i.in_transit_qty>0) ORDER BY l.id`

const mapSummary business.Statement = `WITH visible_locations AS (
 SELECT l.id FROM material_dispatch.locations l
 WHERE l.deleted_at IS NULL AND l.status=1
 AND ($2='' OR strpos(l.name,$2)>0 OR strpos(coalesce(l.address,''),$2)>0)
 AND ($3='' OR l.id::text=$3)
), visible_stock AS (
 SELECT i.location_id,i.spec_id,i.normal_qty,i.damaged_qty,i.in_transit_qty
 FROM material_dispatch.material_inventories i
 JOIN visible_locations l ON l.id=i.location_id
 JOIN material_dispatch.material_specs s ON s.id=i.spec_id AND s.deleted_at IS NULL
 WHERE ($1='' OR i.spec_id::text=$1)
), totals AS (
 SELECT count(DISTINCT spec_id) FILTER (WHERE normal_qty>0) AS normal_spec_count,
 count(DISTINCT spec_id) FILTER (WHERE damaged_qty>0) AS damaged_spec_count,
 count(DISTINCT spec_id) FILTER (WHERE in_transit_qty>0) AS in_transit_spec_count,
 CASE WHEN $1<>'' THEN coalesce(sum(normal_qty),0)::bigint END AS normal_qty,
 CASE WHEN $1<>'' THEN coalesce(sum(damaged_qty),0)::bigint END AS damaged_qty,
 CASE WHEN $1<>'' THEN coalesce(sum(in_transit_qty),0)::bigint END AS in_transit_qty
 FROM visible_stock
)
SELECT (SELECT count(*) FROM visible_locations l WHERE $1='' OR EXISTS (
 SELECT 1 FROM visible_stock i WHERE i.location_id=l.id AND (i.normal_qty>0 OR i.damaged_qty>0 OR i.in_transit_qty>0)
)) AS location_count, totals.*,
(SELECT unit FROM material_dispatch.material_specs WHERE id::text=$1 AND deleted_at IS NULL) AS unit
FROM totals
WHERE $1='' OR EXISTS (SELECT 1 FROM material_dispatch.material_specs WHERE id::text=$1 AND deleted_at IS NULL)`
