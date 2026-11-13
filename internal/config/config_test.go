package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, key := range []string{"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "DB_MAX_CONNS"} {
		t.Setenv(key, "")
	}
	t.Setenv("DATABASE_URL", "postgres://app:secret@localhost/material_dispatch?sslmode=disable")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" || cfg.DBMaxConns != 10 || cfg.Environment != "development" {
		t.Fatalf("unexpected defaults: %+v", cfg.HTTPAddr)
	}
	for _, item := range []struct{ key, value string }{
		{"DATABASE_URL", ""}, {"DATABASE_URL", "password=secret malformed"},
		{"APP_ENV", "invalid"}, {"HTTP_ADDR", "localhost"}, {"HTTP_ADDR", "localhost:70000"},
		{"LOG_LEVEL", "nope"}, {"DB_MAX_CONNS", "0"}, {"DB_MAX_CONNS", "bad"},
	} {
		t.Run(item.key+item.value, func(t *testing.T) {
			t.Setenv(item.key, item.value)
			_, err := Load()
			if err == nil {
				t.Fatal("expected configuration validation error")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestProductionRequiresHTTPSOrigin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/material_dispatch")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_ORIGIN", "")
	if _, err := Load(); err == nil {
		t.Fatal("production must require explicit HTTPS origin")
	}
	t.Setenv("APP_ORIGIN", "http://example.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure production origin accepted")
	}
	t.Setenv("APP_ORIGIN", "https://example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Origin != "https://example.com" || cfg.UploadDir == "" {
		t.Fatal("missing web settings")
	}
}

func TestDevelopmentOriginMatchesViteHost(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/material_dispatch")
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_ORIGIN", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Origin != "http://127.0.0.1:5176" {
		t.Fatal("origin does not match Vite address", cfg.Origin)
	}
}

func TestRouteConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/material_dispatch")
	t.Setenv("TIANDITU_SERVER_KEY", " server-key ")
	t.Setenv("ROUTE_REFRESH_HOUR", "")
	t.Setenv("ROUTE_DAILY_LIMIT", "")
	cfg, err := Load()
	if err != nil || cfg.TiandituKey != "server-key" || cfg.RouteRefreshHour != 2 || cfg.RouteDailyLimit != 1000 {
		t.Fatal("routing config defaults wrong", err)
	}
	for _, v := range []struct{ key, value string }{{"ROUTE_REFRESH_HOUR", "24"}, {"ROUTE_REFRESH_HOUR", "-1"}, {"ROUTE_DAILY_LIMIT", "0"}, {"ROUTE_DAILY_LIMIT", "invalid"}} {
		t.Run(v.key+v.value, func(t *testing.T) {
			t.Setenv(v.key, v.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid route configuration accepted")
			}
		})
	}
}
