package auth

import (
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Username, &user.RealName, &user.Phone, &user.Role, &user.Status)
	return user, err
}

const columns = `id,username,real_name,coalesce(phone,''),role,status`

// SQL stays in the domain repository; services own validation, state rules and transactions.
const sessionUser business.Statement = `SELECT u.id,u.username,u.real_name,coalesce(u.phone,''),u.role,u.status,f.csrf_token,s.expires_at,f.expires_at FROM material_dispatch.sessions s JOIN material_dispatch.session_families f ON f.id=s.family_id JOIN material_dispatch.sys_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND f.expires_at>now() AND u.status=1 AND u.deleted_at IS NULL`

const loginUser business.Statement = `SELECT ` + columns + `,password_hash,failed_login_attempts,locked_until FROM material_dispatch.sys_users WHERE username=$1 AND deleted_at IS NULL FOR UPDATE`

const recordFailedLogin business.Statement = `UPDATE material_dispatch.sys_users SET failed_login_attempts=failed_login_attempts+1,locked_until=CASE WHEN failed_login_attempts+1>=5 THEN now()+interval '15 minutes' ELSE NULL END WHERE id=$1`

const recordSuccessfulLogin business.Statement = `UPDATE material_dispatch.sys_users SET failed_login_attempts=0,locked_until=NULL,last_login_at=now() WHERE id=$1`

const insertSession business.Statement = `INSERT INTO material_dispatch.sessions(token_hash,user_id,expires_at,family_id) VALUES($1,$2,$3,$4)`

const deleteSession business.Statement = `DELETE FROM material_dispatch.session_families WHERE id=(SELECT family_id FROM material_dispatch.sessions WHERE token_hash=$1)`

const lockedPassword business.Statement = `SELECT password_hash FROM material_dispatch.sys_users WHERE id=$1 FOR UPDATE`

const updatePassword business.Statement = `UPDATE material_dispatch.sys_users SET password_hash=$1 WHERE id=$2`

const deleteUserSessions business.Statement = `WITH revoked AS (DELETE FROM material_dispatch.session_families WHERE user_id=$1) DELETE FROM material_dispatch.sessions WHERE user_id=$1`

const insertUser business.Statement = `INSERT INTO material_dispatch.sys_users(username,password_hash,real_name,phone,role,status) VALUES($1,$2,$3,$4,$5,$6) RETURNING ` + columns

const lockUserAdministration business.Statement = `SELECT pg_advisory_xact_lock(hashtext('material_dispatch.users'))`

const lockUser business.Statement = `SELECT ` + columns + ` FROM material_dispatch.sys_users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`

const enabledAdminCount business.Statement = `SELECT count(*) FROM material_dispatch.sys_users WHERE role='admin' AND status=1 AND deleted_at IS NULL`

const updateUser business.Statement = `UPDATE material_dispatch.sys_users SET real_name=$1,phone=$2,role=$3,status=$4,password_hash=CASE WHEN $5='' THEN password_hash ELSE $5 END,failed_login_attempts=0,locked_until=NULL WHERE id=$6 RETURNING ` + columns

var listUsers = business.Listing{Select: `SELECT id::text,username,real_name,coalesce(phone,'') AS phone,role,status,last_login_at,created_at`, From: `FROM material_dispatch.sys_users WHERE deleted_at IS NULL AND ($1='' OR username ILIKE '%'||$1||'%' OR real_name ILIKE '%'||$1||'%')`}

var listContacts = business.Listing{Select: `SELECT id::text,real_name AS name`, From: `FROM material_dispatch.sys_users WHERE status=1 AND deleted_at IS NULL AND ($1='' OR real_name ILIKE '%'||$1||'%' OR username ILIKE '%'||$1||'%')`}

const insertFamily business.Statement = `INSERT INTO material_dispatch.session_families(id,user_id,csrf_token,expires_at) VALUES($1,$2,$3,$4)`
const insertRefresh business.Statement = `INSERT INTO material_dispatch.refresh_tokens(token_hash,family_id) VALUES($1,$2)`
const refreshUserID business.Statement = `SELECT f.user_id FROM material_dispatch.refresh_tokens r JOIN material_dispatch.session_families f ON f.id=r.family_id WHERE r.token_hash=$1`
const lockedRefresh business.Statement = `SELECT f.id,f.csrf_token,f.expires_at,r.used_at FROM material_dispatch.refresh_tokens r JOIN material_dispatch.session_families f ON f.id=r.family_id WHERE r.token_hash=$1 FOR UPDATE OF f,r`
const consumeRefresh business.Statement = `UPDATE material_dispatch.refresh_tokens SET used_at=now() WHERE token_hash=$1`
const revokeFamily business.Statement = `DELETE FROM material_dispatch.session_families WHERE id=$1`
