//go:build integration

package integration

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivateValidatedUploads(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	upload := func(filename string, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
		_ = writer.Close()
		request := httptest.NewRequest("POST", "http://localhost/api/v1/uploads", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("Origin", "http://localhost")
		request.Header.Set("X-CSRF-Token", client.csrf)
		request.AddCookie(client.cookie)
		response := httptest.NewRecorder()
		client.router.ServeHTTP(response, request)
		return response
	}
	requireStatus(t, upload("fake.png", []byte("<script>alert(1)</script>")), 400)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data := requireStatus(t, upload("proof.png", encoded.Bytes()), 201)
	path := data["url"].(string)
	response := client.call("GET", path, nil)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), encoded.Bytes()) {
		t.Fatal("uploaded image could not be read", response.Code)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing content protection")
	}
	anonymous := &apiClient{router: client.router}
	requireStatus(t, anonymous.call("GET", path, nil), 401)
	requireStatus(t, client.call("POST", "/api/v1/damage-reports", map[string]any{"location_id": "1", "spec_id": "1", "damaged_qty": 1, "damage_level": "minor", "description": "invalid", "image_urls": []string{"https://evil.example/payload.svg"}}), 400)
}
func TestOriginAndStrictJSON(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	request := httptest.NewRequest("POST", "http://localhost/api/v1/auth/logout", bytes.NewBufferString("{}"))
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("X-CSRF-Token", client.csrf)
	request.AddCookie(client.cookie)
	response := httptest.NewRecorder()
	client.router.ServeHTTP(response, request)
	requireStatus(t, response, 403)
	requireStatus(t, client.call("POST", "/api/v1/locations", map[string]any{"unexpected": true}), 400)
	requireStatus(t, client.call("GET", "/api/v1/locations?page=-1", nil), 400)
}
