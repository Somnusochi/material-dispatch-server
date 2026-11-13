//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
)

type tokenPair struct {
	access, refresh *http.Cookie
	csrf            string
}

func readPair(t *testing.T, r *httptest.ResponseRecorder) tokenPair {
	t.Helper()
	data := requireStatus(t, r, 200)
	p := tokenPair{csrf: data["csrf_token"].(string)}
	for _, c := range r.Result().Cookies() {
		switch c.Name {
		case "material_dispatch_session":
			p.access = c
		case "material_dispatch_refresh":
			p.refresh = c
		}
	}
	if p.access == nil || p.refresh == nil {
		t.Fatal("login/refresh must issue distinct access and refresh cookies")
	}
	for _, c := range []*http.Cookie{p.access, p.refresh} {
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
			t.Fatal("unsafe cookie")
		}
		if strings.Contains(r.Body.String(), c.Value) {
			t.Fatal("raw token leaked in JSON")
		}
	}
	if p.access.Value == p.refresh.Value || p.access.Path != "/api/v1" || p.refresh.Path != "/api/v1/auth" {
		t.Fatal("token isolation failed")
	}
	if _, ok := data["access_expires_at"].(string); !ok {
		t.Fatal("missing access expiration")
	}
	return p
}
func refreshCall(client *apiClient, token *http.Cookie, origin, marker string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "http://localhost/api/v1/auth/refresh", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Refresh-Request", marker)
	if token != nil {
		req.AddCookie(token)
	}
	r := httptest.NewRecorder()
	client.router.ServeHTTP(r, req)
	return r
}
func signInPair(t *testing.T, c *apiClient, name, password string) tokenPair {
	t.Helper()
	return readPair(t, c.call("POST", "/api/v1/auth/login", map[string]string{"username": name, "password": password}))
}
func TestDualTokenLifecycle(t *testing.T) {
	pool := testPool(t)
	c := newClient(t, pool)
	name, password := createAdmin(t, pool)
	p := signInPair(t, c, name, password)
	if p.access.MaxAge != 86400 || p.refresh.MaxAge != 604800 {
		t.Fatalf("expected access 24h, refresh 7d; got %d/%d", p.access.MaxAge, p.refresh.MaxAge)
	}
	c.cookie = p.access
	c.csrf = p.csrf
	requireStatus(t, c.call("GET", "/api/v1/auth/me", nil), 200)
	if _, err := pool.Exec(context.Background(), `UPDATE material_dispatch.sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, auth.HashToken(p.access.Value)); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, c.call("GET", "/api/v1/locations", nil), 401)
	next := readPair(t, refreshCall(c, p.refresh, "http://localhost", "1"))
	if next.access.Value == p.access.Value || next.refresh.Value == p.refresh.Value || next.csrf != p.csrf {
		t.Fatal("rotation or stable CSRF failed")
	}
	c.cookie = next.access
	requireStatus(t, c.call("GET", "/api/v1/locations", nil), 200)
	// Replay of the consumed refresh token revokes the new access and refresh.
	requireStatus(t, refreshCall(c, p.refresh, "http://localhost", "1"), 401)
	requireStatus(t, c.call("GET", "/api/v1/locations", nil), 401)
	requireStatus(t, refreshCall(c, next.refresh, "http://localhost", "1"), 401)
}
func TestRefreshOriginIsolationAndAbsoluteExpiry(t *testing.T) {
	pool := testPool(t)
	c := newClient(t, pool)
	name, password := createAdmin(t, pool)
	p := signInPair(t, c, name, password)
	for _, x := range [][2]string{{"", "1"}, {"http://evil.invalid", "1"}, {"http://localhost", ""}} {
		requireStatus(t, refreshCall(c, p.refresh, x[0], x[1]), 403)
	}
	requireStatus(t, refreshCall(c, &http.Cookie{Name: p.refresh.Name, Value: p.access.Value}, "http://localhost", "1"), 401)
	c.cookie = &http.Cookie{Name: p.access.Name, Value: p.refresh.Value}
	requireStatus(t, c.call("GET", "/api/v1/locations", nil), 401)
	if _, err := pool.Exec(context.Background(), `UPDATE material_dispatch.session_families SET expires_at=now()+interval '1 hour' WHERE id=(SELECT family_id FROM material_dispatch.sessions WHERE token_hash=$1)`, auth.HashToken(p.access.Value)); err != nil {
		t.Fatal(err)
	}
	n := readPair(t, refreshCall(c, p.refresh, "http://localhost", "1"))
	if n.refresh.MaxAge > 3600 || n.access.MaxAge > 3600 {
		t.Fatal("rotation must not extend absolute expiry")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE material_dispatch.session_families SET expires_at=now()-interval '1 second' WHERE id=(SELECT family_id FROM material_dispatch.sessions WHERE token_hash=$1)`, auth.HashToken(n.access.Value)); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, refreshCall(c, n.refresh, "http://localhost", "1"), 401)
}
func TestBothTokensRevokedOnLogoutPasswordAndUserChanges(t *testing.T) {
	pool := testPool(t)
	for _, action := range []string{"logout", "password", "disable", "role", "reset"} {
		t.Run(action, func(t *testing.T) {
			c := newClient(t, pool)
			name, password := createAdmin(t, pool)
			p := signInPair(t, c, name, password)
			c.cookie = p.access
			c.csrf = p.csrf
			switch action {
			case "logout":
				requireStatus(t, c.call("POST", "/api/v1/auth/logout", map[string]string{}), 200)
			case "password":
				requireStatus(t, c.call("POST", "/api/v1/auth/password", map[string]string{"current_password": password, "new_password": "Different-test-pass-2026"}), 200)
			default:
				// A second administrator edits the subject without bypassing business rules.
				other := newClient(t, pool)
				admin, pass := createAdmin(t, pool)
				login(t, other, admin, pass)
				me := requireStatus(t, c.call("GET", "/api/v1/auth/me", nil), 200)
				u := me["user"].(map[string]any)
				input := map[string]any{"real_name": "测试管理员", "role": "admin", "status": 1}
				if action == "disable" {
					input["status"] = 0
				}
				if action == "role" {
					input["role"] = "inspector"
				}
				if action == "reset" {
					input["password"] = "Reset-test-password-2026"
				}
				requireStatus(t, other.call("PUT", "/api/v1/users/"+u["id"].(string), input), 200)
			}
			requireStatus(t, c.call("GET", "/api/v1/auth/me", nil), 401)
			requireStatus(t, refreshCall(c, p.refresh, "http://localhost", "1"), 401)
		})
	}
}
func TestLoginExpiryResponseMatches24Hours(t *testing.T) {
	pool := testPool(t)
	c := newClient(t, pool)
	name, password := createAdmin(t, pool)
	r := c.call("POST", "/api/v1/auth/login", map[string]string{"username": name, "password": password})
	var body struct {
		Data struct {
			Access  time.Time `json:"access_expires_at"`
			Refresh time.Time `json:"refresh_expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if time.Until(body.Data.Access) < 24*time.Hour-time.Minute || time.Until(body.Data.Refresh) < 7*24*time.Hour-time.Minute {
		t.Fatal("expected 24 hour access and 7 day refresh expiry")
	}
}
