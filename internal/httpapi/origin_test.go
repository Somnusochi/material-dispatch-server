package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginOriginPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, configured, incoming string
		development, secure        bool
		allowed                    bool
	}{
		{name: "development canonical", configured: "http://127.0.0.1:5176", incoming: "http://127.0.0.1:5176", development: true, allowed: true},
		{name: "development localhost alias", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5176", development: true, allowed: true},
		{name: "development reverse alias", configured: "http://localhost:5176", incoming: "http://127.0.0.1:5176", development: true, allowed: true},
		{name: "same protocol required", configured: "http://127.0.0.1:5176", incoming: "https://localhost:5176", development: true},
		{name: "same port required", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5177", development: true},
		{name: "hostname suffix rejected", configured: "http://127.0.0.1:5176", incoming: "http://localhost.evil:5176", development: true},
		{name: "external origin rejected", configured: "http://127.0.0.1:5176", incoming: "https://evil.example", development: true},
		{name: "opaque origin rejected", configured: "http://127.0.0.1:5176", incoming: "null", development: true},
		{name: "origin with path rejected", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5176/", development: true},
		{name: "origin with query rejected", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5176?foo=1", development: true},
		{name: "origin list rejected", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5176 https://evil.example", development: true},
		{name: "production canonical", configured: "https://system.example", incoming: "https://system.example", secure: true, allowed: true},
		{name: "production loopback alias rejected", configured: "https://127.0.0.1:5176", incoming: "https://localhost:5176", secure: true},
		{name: "nondevelopment alias rejected", configured: "http://127.0.0.1:5176", incoming: "http://localhost:5176"},
		{name: "secure development alias rejected", configured: "https://127.0.0.1:5176", incoming: "https://localhost:5176", development: true, secure: true},
		{name: "custom development origin canonical", configured: "https://dev.example:5176", incoming: "https://dev.example:5176", development: true, allowed: true},
		{name: "custom development does not trust loopback", configured: "https://dev.example:5176", incoming: "https://localhost:5176", development: true},
		{name: "nonbrowser client unchanged", configured: "http://127.0.0.1:5176", development: true, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewApplication(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Origin: tc.configured, Development: tc.development, SecureCookies: tc.secure})
			request := httptest.NewRequest("POST", "http://evil.example/api/v1/auth/login", strings.NewReader("{"))
			request.Header.Set("Content-Type", "application/json")
			if tc.incoming != "" {
				request.Header.Set("Origin", tc.incoming)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			expected := 403
			if tc.allowed {
				expected = 400
			} // 已通过来源检查，停在无效JSON，避免触及数据库。
			if response.Code != expected {
				t.Fatalf("want status %d, got %d: %s", expected, response.Code, response.Body.String())
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("must not add broad CORS")
			}
			if !tc.allowed {
				var body struct {
					Error struct{ Code, Message string } `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error.Code != "ORIGIN_NOT_ALLOWED" || body.Error.Message != "当前访问地址未获允许，请使用配置的系统地址" {
					t.Fatalf("unclear origin error: %+v", body.Error)
				}
			}
		})
	}
}
