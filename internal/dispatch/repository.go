package dispatch

import "github.com/Somnusochi/material-dispatch-server/internal/platform/business"

// SQL stays in the domain repository; services own validation, state rules and transactions.
const selectOrder = `SELECT d.id::text,d.order_no,d.from_location_id::text,f.name AS from_location_name,d.to_location_id::text,t.name AS to_location_name,d.spec_id::text,s.name AS spec_name,s.unit,d.dispatch_qty,d.actual_received_qty,d.status,d.priority,d.creator_id::text,u.real_name AS creator_name,d.dispatcher_id::text,d.receiver_id::text,d.departed_at,d.completed_at,coalesce(d.remark,'') AS remark,d.created_at,d.updated_at`

const orderFrom = `FROM material_dispatch.dispatch_orders d JOIN material_dispatch.locations f ON f.id=d.from_location_id JOIN material_dispatch.locations t ON t.id=d.to_location_id JOIN material_dispatch.material_specs s ON s.id=d.spec_id JOIN material_dispatch.sys_users u ON u.id=d.creator_id`

var listOrders = business.Listing{Select: selectOrder, From: orderFrom + ` WHERE d.deleted_at IS NULL AND ($1='' OR d.status=$1) AND ($2='' OR d.order_no ILIKE '%'||$2||'%' OR f.name ILIKE '%'||$2||'%' OR t.name ILIKE '%'||$2||'%') AND ($3='' OR d.from_location_id::text=$3 OR d.to_location_id::text=$3) AND ($4='' OR d.spec_id::text=$4)`}

const findOrder business.Statement = selectOrder + ` ` + orderFrom + ` WHERE d.id=$1 AND d.deleted_at IS NULL`

const insertOrder business.Statement = `INSERT INTO material_dispatch.dispatch_orders(order_no,from_location_id,to_location_id,spec_id,dispatch_qty,priority,creator_id,remark) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`

const lockOrder business.Statement = `SELECT from_location_id,to_location_id,spec_id,dispatch_qty,status FROM material_dispatch.dispatch_orders WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`

const cancelOrder business.Statement = `UPDATE material_dispatch.dispatch_orders SET status='cancelled' WHERE id=$1`

const departOrder business.Statement = `UPDATE material_dispatch.dispatch_orders SET status='in_transit',dispatcher_id=$1,departed_at=now() WHERE id=$2`

const receiveOrder business.Statement = `UPDATE material_dispatch.dispatch_orders SET status='completed',actual_received_qty=$1,receiver_id=$2,completed_at=now() WHERE id=$3`

const insertTransitDamage business.Statement = `INSERT INTO material_dispatch.damage_reports(report_no,location_id,spec_id,dispatch_order_id,damaged_qty,description,image_urls,status,reporter_id,reviewer_id,reviewed_at,review_remark) VALUES($1,$2,$3,$4,$5,$6,$7,'confirmed',$8,$8,now(),'运输签收自动确认，已计入损毁库存') RETURNING id`
