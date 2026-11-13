package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubDB struct {
	err      error
	calls    int
	deadline bool
}

func (d *stubDB) Ping(ctx context.Context) error {
	d.calls++
	_, d.deadline = ctx.Deadline()
	return d.err
}
func TestHealthAndErrors(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		unavailable  bool
		status       int
		code         string
	}{
		{"GET", "/healthz", true, 200, ""},
		{"GET", "/readyz", false, 200, ""},
		{"GET", "/readyz", true, 503, "DATABASE_UNAVAILABLE"},
		{"GET", "/api/v1/system/status", false, 200, ""},
		{"GET", "/api/v1/system/status", true, 503, "DATABASE_UNAVAILABLE"},
		{"GET", "/missing", false, 404, "NOT_FOUND"},
		{"POST", "/healthz", false, 405, "METHOD_NOT_ALLOWED"},
	} {
		t.Run(tc.method+tc.path+tc.code, func(t *testing.T) {
			db := &stubDB{}
			if tc.unavailable {
				db.err = errors.New("postgres://secret@internal-db")
			}
			router := NewRouter(db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("X-Request-ID", "test-request-123")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status %d: %s", res.Code, res.Body)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if string(body["request_id"]) != `"test-request-123"` || res.Header().Get("X-Request-ID") != "test-request-123" {
				t.Fatal("request ID missing")
			}
			if tc.code != "" && !strings.Contains(string(body["error"]), tc.code) {
				t.Fatal("error code missing", res.Body)
			}
			if strings.Contains(res.Body.String(), "secret") {
				t.Fatal("leaked database error")
			}
			if tc.path == "/healthz" && db.calls != 0 {
				t.Fatal("liveness depends on database")
			}
			if (tc.path == "/readyz" || tc.path == "/api/v1/system/status") && !db.deadline {
				t.Fatal("database check lacks deadline")
			}
			if tc.path == "/api/v1/system/status" && !tc.unavailable && !strings.Contains(string(body["data"]), `"database":"up"`) {
				t.Fatal("missing system data")
			}
		})
	}
}
func TestRecoveryAndRequestID(t *testing.T) {
	router := NewRouter(&stubDB{}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	router.GET("/panic", func(_ *gin.Context) { panic("secret panic") })
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("X-Request-ID", strings.Repeat("x", 256))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != 500 || !strings.Contains(res.Body.String(), "INTERNAL_ERROR") || strings.Contains(res.Body.String(), "secret") {
		t.Fatal(res.Body)
	}
	if id := res.Header().Get("X-Request-ID"); len(id) == 0 || len(id) > 64 {
		t.Fatal("invalid generated request ID")
	}
}
