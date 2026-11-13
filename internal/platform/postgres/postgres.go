package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// Open verifies connectivity before returning; errors deliberately omit the DSN.
func Open(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid database connection configuration")
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "material-dispatch-server"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("unable to initialize database pool")
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database connection unavailable")
	}
	return pool, nil
}
