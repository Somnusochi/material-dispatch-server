package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string      { return e.Message }
func Invalid(message string) error  { return &Error{http.StatusBadRequest, "VALIDATION_ERROR", message} }
func Conflict(message string) error { return &Error{http.StatusConflict, "CONFLICT", message} }
func NotFound() error               { return &Error{http.StatusNotFound, "NOT_FOUND", "记录不存在"} }
func Unauthorized() error           { return &Error{http.StatusUnauthorized, "UNAUTHORIZED", "请重新登录"} }
func Forbidden() error              { return &Error{http.StatusForbidden, "FORBIDDEN", "没有此操作权限"} }
func Normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound()
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return Conflict("编码或名称已存在")
		case "23503":
			return Invalid("关联记录不存在或已被使用")
		case "23514", "22003", "22P02":
			return Invalid("提交的数据不符合约束")
		case "40001", "40P01":
			return Conflict("数据正在被其他操作修改，请刷新重试")
		}
	}
	return err
}

type Actor struct {
	ID                                          int64
	Username, Role, RequestID, Method, Path, IP string
}
type Row map[string]any
type Queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Rows(ctx context.Context, db Queryer, query string, args ...any) ([]Row, error) {
	rows, err := db.Query(ctx, "SELECT row_to_json(result) FROM ("+query+") result", args...)
	if err != nil {
		return nil, Normalize(err)
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item Row
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, Normalize(rows.Err())
}
func One(ctx context.Context, db Queryer, query string, args ...any) (Row, error) {
	rows, err := Rows(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, NotFound()
	}
	return rows[0], nil
}
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return Normalize(err)
	}
	return Normalize(tx.Commit(ctx))
}
func ID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, Invalid("无效的记录 ID")
	}
	return id, nil
}
func RequireText(value, label string, max int) error {
	if strings.TrimSpace(value) == "" || len([]rune(value)) > max {
		return Invalid(fmt.Sprintf("%s不能为空且不能超过%d字", label, max))
	}
	return nil
}

type Page struct {
	Page, Size                                          int
	Keyword, Status, LocationID, SpecID, Module, Action string
}
type List struct {
	Items    []Row `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func Paginate(ctx context.Context, db Queryer, selectSQL, fromSQL string, args []any, page Page) (List, error) {
	result := List{Items: []Row{}, Page: page.Page, PageSize: page.Size}
	if err := db.QueryRow(ctx, "SELECT count(*) "+fromSQL, args...).Scan(&result.Total); err != nil {
		return result, Normalize(err)
	}
	args = append(args, page.Size, (page.Page-1)*page.Size)
	rows, err := Rows(ctx, db, "SELECT * FROM ("+selectSQL+" "+fromSQL+") page_result"+fmt.Sprintf(" ORDER BY page_result.id::bigint DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	result.Items = rows
	return result, err
}
