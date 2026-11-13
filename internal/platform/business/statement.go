package business

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Statement is repository-owned SQL. Values are always passed as parameters.
// Queryer is shared only because pgx pools and transactions are substitutable here.
type Statement string

func (s Statement) Exec(ctx context.Context, db Queryer, args ...any) (pgconn.CommandTag, error) {
	return db.Exec(ctx, string(s), args...)
}
func (s Statement) Row(ctx context.Context, db Queryer, args ...any) pgx.Row {
	return db.QueryRow(ctx, string(s), args...)
}
func (s Statement) One(ctx context.Context, db Queryer, args ...any) (Row, error) {
	return One(ctx, db, string(s), args...)
}
func (s Statement) Rows(ctx context.Context, db Queryer, args ...any) ([]Row, error) {
	return Rows(ctx, db, string(s), args...)
}

type Listing struct{ Select, From string }

func (s Listing) Paginate(ctx context.Context, db Queryer, args []any, page Page) (List, error) {
	return Paginate(ctx, db, s.Select, s.From, args, page)
}
