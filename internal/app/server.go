package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}

// Serve drains in-flight requests when the application context is cancelled.
func Serve(ctx context.Context, server *http.Server, listener net.Listener, grace time.Duration, logger *slog.Logger) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		logger.Info("server shutting down")
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-done
			return err
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
