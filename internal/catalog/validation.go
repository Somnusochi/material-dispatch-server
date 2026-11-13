package catalog

import (
	"context"
	"strings"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/jackc/pgx/v5"
)

// Active locks catalogue rows until the business transaction commits, preventing concurrent deactivation.
func Active(ctx context.Context, tx pgx.Tx, locations []int64, spec int64) error {
	for _, id := range locations {
		var status int
		if err := activeLocation.Row(ctx, tx, id).Scan(&status); err != nil {
			return business.Normalize(err)
		}
		if status != 1 {
			return business.Conflict("地点已停用")
		}
	}
	var id int64
	return business.Normalize(activeSpec.Row(ctx, tx, spec).Scan(&id))
}
func CheckImages(ctx context.Context, q business.Queryer, urls []string) error {
	if len(urls) > 6 {
		return business.Invalid("最多上传6张照片")
	}
	for _, url := range urls {
		if !strings.HasPrefix(url, "/api/v1/uploads/") {
			return business.Invalid("图片必须先上传")
		}
		id := strings.TrimPrefix(url, "/api/v1/uploads/")
		var exists bool
		if err := uploadExists.Row(ctx, q, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return business.Invalid("图片不存在")
		}
	}
	return nil
}
