package damage

import "github.com/Somnusochi/material-dispatch-server/internal/platform/business"

// SQL stays in the domain repository; services own validation, state rules and transactions.
const selectReport = `SELECT r.id::text,r.report_no,r.location_id::text,l.name AS location_name,r.spec_id::text,s.name AS spec_name,s.unit,r.dispatch_order_id::text,r.damaged_qty,r.damage_level,r.description,r.image_urls,r.status,r.reporter_id::text,u.real_name AS reporter_name,r.reviewer_id::text,r.reviewed_at,r.review_remark,r.created_at,r.updated_at`

const reportFrom = `FROM material_dispatch.damage_reports r JOIN material_dispatch.locations l ON l.id=r.location_id JOIN material_dispatch.material_specs s ON s.id=r.spec_id JOIN material_dispatch.sys_users u ON u.id=r.reporter_id`

var listReports = business.Listing{Select: selectReport, From: reportFrom + ` WHERE r.deleted_at IS NULL AND ($1='' OR r.status=$1) AND ($2='' OR r.report_no ILIKE '%'||$2||'%' OR l.name ILIKE '%'||$2||'%' OR s.name ILIKE '%'||$2||'%') AND ($3='' OR r.location_id::text=$3) AND ($4='' OR r.spec_id::text=$4)`}

const findReport business.Statement = selectReport + ` ` + reportFrom + ` WHERE r.id=$1 AND r.deleted_at IS NULL`

const insertReport business.Statement = `INSERT INTO material_dispatch.damage_reports(report_no,location_id,spec_id,damaged_qty,damage_level,description,image_urls,reporter_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`

const lockReport business.Statement = `SELECT location_id,spec_id,damaged_qty,status FROM material_dispatch.damage_reports WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`

const updateReport business.Statement = `UPDATE material_dispatch.damage_reports SET status=$1,reviewer_id=$2,reviewed_at=now(),review_remark=$3 WHERE id=$4`
