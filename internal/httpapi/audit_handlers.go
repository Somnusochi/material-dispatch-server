package httpapi

import (
	"context"

	"github.com/Somnusochi/material-dispatch-server/internal/audit"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (a *application) registerAuditRoutes(api *gin.RouterGroup, pool *pgxpool.Pool) {
	api.GET("/operation-logs", roles("admin"), a.list(func(ctx context.Context, p business.Page) (business.List, error) { return audit.List(ctx, pool, p) }))
}
