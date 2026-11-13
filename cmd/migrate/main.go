package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Somnusochi/material-dispatch-server/internal/config"
	"github.com/Somnusochi/material-dispatch-server/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"log/slog"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "status") {
		return errors.New("usage: migrate up|down|status")
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("unable to read .env")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return errors.New("database connection unavailable")
	}
	provider, err := migrations.New(db)
	if err != nil {
		return err
	}
	switch os.Args[1] {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Applied %d migration(s)\n", len(results))
	case "down":
		_, err := provider.Down(ctx)
		return err
	case "status":
		results, err := provider.Status(ctx)
		if err != nil {
			return err
		}
		for _, result := range results {
			fmt.Printf("%d %s\n", result.Source.Version, result.State)
		}
	}
	return nil
}
