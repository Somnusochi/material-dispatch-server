package upload

import "github.com/Somnusochi/material-dispatch-server/internal/platform/business"

// SQL stays in the domain repository; services own validation, state rules and transactions.
const insertFile business.Statement = `INSERT INTO material_dispatch.uploads(id,owner_id,storage_key,original_name,content_type,size_bytes) VALUES($1,$2,$3,$4,$5,$6)`

const loadFile business.Statement = `SELECT storage_key,content_type FROM material_dispatch.uploads WHERE id=$1`
