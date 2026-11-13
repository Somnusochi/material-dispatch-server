package httpapi

import (
	"log/slog"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/Somnusochi/material-dispatch-server/internal/damage"
	"github.com/Somnusochi/material-dispatch-server/internal/dispatch"
	"github.com/Somnusochi/material-dispatch-server/internal/inventory"
	"github.com/Somnusochi/material-dispatch-server/internal/routing"
	"github.com/Somnusochi/material-dispatch-server/internal/upload"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Origin, UploadDir string
	SecureCookies     bool
	Development       bool
	Routes            *routing.Service
}
type application struct {
	auth      *auth.Service
	catalog   *catalog.Service
	inventory *inventory.Service
	dispatch  *dispatch.Service
	damage    *damage.Service
	upload    *upload.Service
	logger    *slog.Logger
	options   Options
}

func NewApplication(pool *pgxpool.Pool, logger *slog.Logger, options Options) *gin.Engine {
	if options.Routes == nil {
		options.Routes = routing.NewService(pool, nil, routing.Options{RefreshHour: 2})
	}
	a := &application{
		auth: auth.NewService(pool), catalog: catalog.NewService(pool),
		inventory: inventory.NewService(pool), dispatch: dispatch.NewService(pool),
		damage: damage.NewService(pool), upload: upload.NewService(pool, options.UploadDir),
		logger: logger, options: options,
	}
	router := NewRouter(pool, logger)
	origins := trustedOrigins(options)
	api := router.Group("/api/v1")
	api.Use(requestPolicy(origins))
	a.registerPublicAuth(api, origins)
	api.Use(a.authenticate)
	a.registerAccountRoutes(api)
	a.registerUserRoutes(api)
	a.registerLocationRoutes(api)
	a.registerSpecRoutes(api)
	a.registerMapRoutes(api)
	a.registerInventoryRoutes(api)
	a.registerDispatchRoutes(api)
	a.registerDamageRoutes(api)
	a.registerAuditRoutes(api, pool)
	a.registerUploadRoutes(api)
	return router
}
