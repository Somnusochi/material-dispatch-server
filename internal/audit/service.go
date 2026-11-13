package audit

import (
	"context"
	"encoding/json"
	"net"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Record(ctx context.Context, tx pgx.Tx, actor business.Actor, module, action, target, summary string, before, after any) error {
	var old, newData []byte
	var err error
	if before != nil {
		old, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		newData, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	var userID any
	if actor.ID > 0 {
		userID = actor.ID
	}
	var ip any
	if net.ParseIP(actor.IP) != nil {
		ip = actor.IP
	}
	_, err = insertAudit.Exec(ctx, tx, userID, actor.Username, module, action, target, summary, actor.RequestID, actor.Method, actor.Path, ip, old, newData)
	return err
}
func List(ctx context.Context, pool *pgxpool.Pool, p business.Page) (business.List, error) {
	return listLogs(ctx, pool, p)
}
