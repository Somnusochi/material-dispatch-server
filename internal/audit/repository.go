package audit

import (
	"context"
	"fmt"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQL stays in the domain repository; services own validation, state rules and transactions.
const insertAudit business.Statement = `INSERT INTO material_dispatch.operation_logs(user_id,username,module,action,target_id,summary,request_id,request_method,request_path,ip_address,before_data,after_data) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`

func listLogs(ctx context.Context, pool *pgxpool.Pool, p business.Page) (business.List, error) {
	from := `FROM material_dispatch.operation_logs WHERE true`
	args := []any{}
	for _, filter := range []struct{ column, value string }{{"module", p.Module}, {"action", p.Action}} {
		if filter.value != "" {
			args = append(args, filter.value)
			from += fmt.Sprintf(" AND %s=$%d", filter.column, len(args))
		}
	}
	if p.Keyword != "" {
		args = append(args, "%"+p.Keyword+"%")
		from += fmt.Sprintf(" AND (summary ILIKE $%d OR username ILIKE $%d)", len(args), len(args))
	}
	return business.Paginate(ctx, pool, `SELECT id::text,user_id::text,username,module,action,target_id,summary,request_id,request_method,request_path,ip_address::text,before_data,after_data,created_at`, from, args, p)
}
