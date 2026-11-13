package main

import (
	"context"
	"errors"
	"github.com/Somnusochi/material-dispatch-server/internal/app"
	"github.com/Somnusochi/material-dispatch-server/internal/config"
	"github.com/Somnusochi/material-dispatch-server/internal/httpapi"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/postgres"
	"github.com/Somnusochi/material-dispatch-server/internal/routing"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("unable to read .env")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	connectCtx, connectCancel := context.WithTimeout(ctx, 5*time.Second)
	pool, err := postgres.Open(connectCtx, cfg.DatabaseURL, cfg.DBMaxConns)
	connectCancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	var provider routing.Provider
	if cfg.TiandituKey != "" {
		provider = routing.NewTianditu(cfg.TiandituKey)
	}
	routes := routing.NewService(pool, provider, routing.Options{RefreshHour: cfg.RouteRefreshHour, DailyLimit: cfg.RouteDailyLimit})
	gin.SetMode(gin.ReleaseMode)
	handler := httpapi.NewApplication(pool, logger, httpapi.Options{
		Origin: cfg.Origin, UploadDir: cfg.UploadDir,
		SecureCookies: cfg.Environment == "production", Development: cfg.Environment == "development",
		Routes: routes,
	})
	server := app.NewServer(cfg.HTTPAddr, handler)
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return err
	}
	logger.Info("server listening", "address", listener.Addr().String(), "environment", cfg.Environment)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); routes.Run(workerCtx, logger) }()
	defer func() { stopWorker(); <-workerDone }()
	return app.Serve(ctx, server, listener, 10*time.Second, logger)
}
