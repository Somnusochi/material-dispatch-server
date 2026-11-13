//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"
)

func specPhoto(t *testing.T, client *apiClient) string {
	t.Helper()
	var picture, body bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "material.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "http://localhost/api/v1/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://localhost")
	request.Header.Set("X-CSRF-Token", client.csrf)
	request.AddCookie(client.cookie)
	response := httptest.NewRecorder()
	client.router.ServeHTTP(response, request)
	return requireStatus(t, response, 201)["url"].(string)
}

func TestSimpleSpecCreate(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	first := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "钢管", "unit": "根"}), 201)
	code, ok := first["spec_code"].(string)
	if !ok || code == "" || len(code) > 64 {
		t.Fatal("missing generated code", first)
	}
	if first["name"] != "钢管" || first["unit"] != "根" || first["category"] != "general" || first["unit_weight_kg"] != float64(0) || first["image_url"] != "" {
		t.Fatal("unexpected generic material", first)
	}
	second := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "水泥", "unit": "袋", "image_url": specPhoto(t, client)}), 201)
	if second["spec_code"] == code || second["image_url"] == "" {
		t.Fatal("new material code/photo missing", second)
	}
}

func TestSimpleSpecUpdatePreservesLegacyFields(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	old := requireStatus(t, client.call("POST", "/api/v1/material-specs", map[string]any{"name": "旧钢管", "unit": "根", "spec_code": fmt.Sprint("OLD-", time.Now().UnixNano()), "category": "steel", "unit_weight_kg": 15.6, "dimensions": "3m", "remark": "历史备注", "image_url": specPhoto(t, client)}), 201)
	path := "/api/v1/material-specs/" + old["id"].(string)
	saved := requireStatus(t, client.call("PUT", path, map[string]any{"name": "新钢管", "unit": "根"}), 200)
	for _, field := range []string{"id", "spec_code", "category", "unit_weight_kg", "dimensions", "remark", "image_url"} {
		if saved[field] != old[field] {
			t.Fatalf("omitted %s was changed: %v -> %v", field, old[field], saved[field])
		}
	}
	if saved["name"] != "新钢管" {
		t.Fatal("name not updated", saved)
	}
	cleared := requireStatus(t, client.call("PUT", path, map[string]any{"name": "新钢管", "unit": "根", "image_url": ""}), 200)
	if cleared["image_url"] != "" || cleared["unit_weight_kg"] != old["unit_weight_kg"] {
		t.Fatal("explicit photo removal lost legacy data", cleared)
	}
}

func TestSimpleSpecValidation(t *testing.T) {
	pool := testPool(t)
	client := newClient(t, pool)
	name, password := createAdmin(t, pool)
	login(t, client, name, password)
	for _, tc := range []struct {
		name   string
		fields map[string]any
	}{
		{"missing_unit", map[string]any{"name": "物料"}},
		{"blank_unit", map[string]any{"name": "物料", "unit": " "}},
		{"blank_name", map[string]any{"name": " ", "unit": "件"}},
		{"external_image", map[string]any{"name": "物料", "unit": "件", "image_url": "https://example.com/photo.jpg"}},
		{"missing_upload", map[string]any{"name": "物料", "unit": "件", "image_url": "/api/v1/uploads/not-present"}},
		{"invalid_optional_weight", map[string]any{"name": "物料", "unit": "件", "unit_weight_kg": -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.fields["spec_code"] = fmt.Sprint("INVALID-", time.Now().UnixNano())
			requireStatus(t, client.call("POST", "/api/v1/material-specs", tc.fields), 400)
		})
	}
}
