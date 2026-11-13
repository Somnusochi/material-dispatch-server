//go:build integration

// e2eserver creates an ephemeral test-only account and starts the real API.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/Somnusochi/material-dispatch-server/internal/app"
	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("e2e server failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	_ = godotenv.Load()
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		return fmt.Errorf("requires dedicated *_test database")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	name := "e2e_" + rand.Text()[:12]
	password := rand.Text() + rand.Text()
	if _, err = auth.NewService(pool).CreateInitialAdmin(ctx, name, password, "浏览器测试管理员"); err != nil {
		return err
	}
	if err = os.MkdirAll(".cache", 0700); err != nil {
		return err
	}
	loginFile := ".cache/e2e-login.json"
	data, _ := json.Marshal(map[string]string{"username": name, "password": password})
	if err = os.WriteFile(loginFile, data, 0600); err != nil {
		return err
	}
	defer os.Remove(loginFile)
	gin.SetMode(gin.ReleaseMode)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	router := httpapi.NewApplication(pool, logger, httpapi.Options{Origin: "http://127.0.0.1:4177", UploadDir: ".cache/e2e-uploads"})
	listener, err := net.Listen("tcp", "127.0.0.1:8081")
	if err != nil {
		return err
	}
	fmt.Println("test-only API ready at 127.0.0.1:8081")
	return app.Serve(ctx, app.NewServer("127.0.0.1:8081", router), listener, 5*time.Second, logger)
}
