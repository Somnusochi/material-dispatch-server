//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func init() { gin.SetMode(gin.TestMode) }

type apiClient struct {
	router http.Handler
	cookie *http.Cookie
	csrf   string
}

func (a *apiClient) call(method, path string, body any) *httptest.ResponseRecorder {
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	if a.cookie != nil {
		request.AddCookie(a.cookie)
	}
	if a.csrf != "" {
		request.Header.Set("X-CSRF-Token", a.csrf)
	}
	response := httptest.NewRecorder()
	a.router.ServeHTTP(response, request)
	return response
}
func requireStatus(t *testing.T, r *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if r.Code != status {
		t.Fatalf("expected %d got %d: %s", status, r.Code, r.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data, _ := body["data"].(map[string]any)
	return data
}
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires dedicated *_test database")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func newClient(t *testing.T, pool *pgxpool.Pool) *apiClient {
	t.Helper()
	router := httpapi.NewApplication(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), httpapi.Options{Origin: "http://localhost", UploadDir: t.TempDir()})
	return &apiClient{router: router}
}
func createAdmin(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	name := "admin_" + time.Now().Format("150405.000000000")
	password := "Strong-test-password-123"
	if _, err := auth.NewService(pool).CreateInitialAdmin(context.Background(), name, password, "测试管理员"); err != nil {
		t.Fatal(err)
	}
	return name, password
}
func login(t *testing.T, client *apiClient, name, password string) {
	t.Helper()
	r := client.call("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": password})
	data := requireStatus(t, r, 200)
	client.csrf = data["csrf_token"].(string)
	cookies := r.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	client.cookie = cookies[0]
	if !client.cookie.HttpOnly || client.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("insecure session cookie")
	}
	if strings.Contains(r.Body.String(), "password_hash") || strings.Contains(r.Body.String(), password) {
		t.Fatal("password leaked")
	}
}
func TestAuthenticationAndPermissions(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	requireStatus(t, client.call("GET", "/api/v1/locations", nil), 401)
	name, password := createAdmin(t, pool)
	requireStatus(t, client.call("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": "wrong"}), 401)
	login(t, client, name, password)
	requireStatus(t, client.call("GET", "/api/v1/auth/me", nil), 200)
	inspector := name + "_inspector"
	r := client.call("POST", "/api/v1/users", map[string]any{"username": inspector, "password": password, "real_name": "巡检员", "role": "inspector", "status": 1})
	user := requireStatus(t, r, 201)
	if _, ok := user["id"].(string); !ok {
		t.Fatal("IDs must be JSON strings")
	}
	saved := client.csrf
	client.csrf = ""
	requireStatus(t, client.call("POST", "/api/v1/users", map[string]any{}), 403)
	client.csrf = saved
	other := newClient(t, pool)
	login(t, other, inspector, password)
	requireStatus(t, other.call("GET", "/api/v1/users", nil), 403)
	requireStatus(t, other.call("GET", "/api/v1/locations", nil), 200)
	requireStatus(t, other.call("POST", "/api/v1/material-specs", map[string]any{}), 403)
	requireStatus(t, client.call("PUT", "/api/v1/users/"+user["id"].(string), map[string]any{"real_name": "巡检员", "role": "inspector", "status": 0}), 200)
	requireStatus(t, other.call("GET", "/api/v1/auth/me", nil), 401)
	requireStatus(t, client.call("POST", "/api/v1/auth/password", map[string]any{"current_password": password, "new_password": "Another-test-password-456"}), 200)
	requireStatus(t, client.call("GET", "/api/v1/auth/me", nil), 401)
	login(t, client, name, "Another-test-password-456")
	requireStatus(t, client.call("POST", "/api/v1/auth/logout", map[string]any{}), 200)
	requireStatus(t, client.call("GET", "/api/v1/auth/me", nil), 401)
}
