package postgres

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpenFailsFastAndRedactsCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pool, err := Open(ctx, "postgres://app:supersecret@127.0.0.1:1/test?sslmode=disable", 2)
	if pool != nil {
		pool.Close()
		t.Fatal("unreachable database returned a pool")
	}
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Fatal("credentials leaked")
	}
}
