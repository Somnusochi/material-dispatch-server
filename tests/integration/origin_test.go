//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Somnusochi/material-dispatch-server/internal/httpapi"
)

func TestDevelopmentLoginOriginsRequireCSRF(t *testing.T) {
	pool := testPool(t)
	name, password := createAdmin(t, pool)
	router := httpapi.NewApplication(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), httpapi.Options{
		Origin: "http://127.0.0.1:5176", Development: true, UploadDir: t.TempDir(),
	})
	call := func(method, path, origin string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		request.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	for _, origin := range []string{"http://127.0.0.1:5176", "http://localhost:5176"} {
		t.Run(origin, func(t *testing.T) {
			response := call("POST", "/api/v1/auth/login", origin, map[string]string{"username": name, "password": password}, nil, "")
			data := requireStatus(t, response, http.StatusOK)
			csrf, ok := data["csrf_token"].(string)
			if !ok || csrf == "" || len(response.Result().Cookies()) != 2 {
				t.Fatal("login must establish session and CSRF token")
			}
			cookie := response.Result().Cookies()[0]
			requireStatus(t, call("GET", "/api/v1/auth/me", origin, nil, cookie, ""), http.StatusOK)
			for _, invalidCSRF := range []string{"", "invalid-token"} {
				requireOriginError(t, call("POST", "/api/v1/auth/logout", origin, nil, cookie, invalidCSRF), "FORBIDDEN")
			}
			requireOriginError(t, call("POST", "/api/v1/auth/logout", "", nil, cookie, ""), "FORBIDDEN")
			for _, forbiddenOrigin := range []string{"https://evil.example", "http://localhost:5177", "null"} {
				requireOriginError(t, call("POST", "/api/v1/auth/logout", forbiddenOrigin, nil, cookie, csrf), "ORIGIN_NOT_ALLOWED")
			}
			requireStatus(t, call("GET", "/api/v1/auth/me", origin, nil, cookie, ""), http.StatusOK)
			requireStatus(t, call("POST", "/api/v1/auth/logout", origin, nil, cookie, csrf), http.StatusOK)
			requireStatus(t, call("GET", "/api/v1/auth/me", origin, nil, cookie, ""), http.StatusUnauthorized)
		})
	}
}

func requireOriginError(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	requireStatus(t, response, http.StatusForbidden)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code {
		t.Fatalf("expected error %s, got %s", code, body.Error.Code)
	}
}
