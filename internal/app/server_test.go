package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerTimeoutsAndGracefulShutdown(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := NewServer("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("finished"))
	}))
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Fatal("HTTP timeouts must be bounded")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, server, listener, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	request := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			defer response.Body.Close()
			b, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				err = readErr
			}
			if string(b) != "finished" {
				t.Errorf("response interrupted: %q", b)
			}
		}
		request <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("server did not start")
	}
	cancel()
	close(release)
	if err := <-request; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown stalled")
	}
}
