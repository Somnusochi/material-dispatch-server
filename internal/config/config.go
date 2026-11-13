package config

import (
	"errors"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment      string
	HTTPAddr         string
	DatabaseURL      string
	DBMaxConns       int32
	LogLevel         slog.Level
	Origin           string
	UploadDir        string
	TiandituKey      string
	RouteRefreshHour int
	RouteDailyLimit  int
}

func Load() (Config, error) {
	cfg := Config{Environment: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", "127.0.0.1:8080"), DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL"))}
	if cfg.Environment != "development" && cfg.Environment != "test" && cfg.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development, test or production")
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || number < 1 || number > 65535 {
		return Config{}, errors.New("HTTP_ADDR must be a valid host:port")
	}
	db, err := url.Parse(cfg.DatabaseURL)
	if err != nil || (db.Scheme != "postgres" && db.Scheme != "postgresql") || db.Hostname() == "" || strings.Trim(db.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with host and database")
	}
	maxConns, err := strconv.ParseInt(value("DB_MAX_CONNS", "10"), 10, 32)
	if err != nil || maxConns < 1 {
		return Config{}, errors.New("DB_MAX_CONNS must be a positive integer")
	}
	cfg.DBMaxConns = int32(maxConns)
	cfg.Origin = value("APP_ORIGIN", "http://127.0.0.1:5176")
	origin, originErr := url.Parse(cfg.Origin)
	if originErr != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return Config{}, errors.New("APP_ORIGIN must be an HTTP origin without path")
	}
	if cfg.Environment == "production" && (os.Getenv("APP_ORIGIN") == "" || origin.Scheme != "https") {
		return Config{}, errors.New("production requires explicit HTTPS APP_ORIGIN")
	}
	cfg.UploadDir = value("UPLOAD_DIR", ".data/uploads")
	cfg.TiandituKey = strings.TrimSpace(os.Getenv("TIANDITU_SERVER_KEY"))
	cfg.RouteRefreshHour, err = strconv.Atoi(value("ROUTE_REFRESH_HOUR", "2"))
	if err != nil || cfg.RouteRefreshHour < 0 || cfg.RouteRefreshHour > 23 {
		return Config{}, errors.New("ROUTE_REFRESH_HOUR must be 0 to 23 (Asia/Shanghai)")
	}
	cfg.RouteDailyLimit, err = strconv.Atoi(value("ROUTE_DAILY_LIMIT", "1000"))
	if err != nil || cfg.RouteDailyLimit < 1 || cfg.RouteDailyLimit > 1000000 {
		return Config{}, errors.New("ROUTE_DAILY_LIMIT must be 1 to 1000000")
	}
	if err = cfg.LogLevel.UnmarshalText([]byte(value("LOG_LEVEL", "info"))); err != nil {
		return Config{}, errors.New("LOG_LEVEL must be debug, info, warn or error")
	}
	return cfg, nil
}
func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
