package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindRequiresSingleJSONObject(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"object", `{"name":"仓库"}`, true},
		{"empty_object", `{}`, true},
		{"whitespace", " \n{\"name\":\"仓库\"}\t ", true},
		{"non_json_leading_space", "\u00a0{}", false},
		{"non_json_trailing_space", "{}\u00a0", false},
		{"null", `null`, false},
		{"array", `[]`, false},
		{"string", `"value"`, false},
		{"number", `1`, false},
		{"boolean", `true`, false},
		{"empty", ``, false},
		{"unknown_field", `{"obsolete":"value"}`, false},
		{"multiple_objects", `{} {}`, false},
		{"trailing_null", `{} null`, false},
		{"malformed", `{"name":`, false},
		{"oversize", `{"name":"` + strings.Repeat("x", 1<<20) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/api/v1/test", strings.NewReader(tc.body))
			c.Set("request_id", "bind-test")
			var input struct {
				Name string `json:"name"`
			}
			if got := (&application{}).bind(c, &input); got != tc.valid {
				t.Fatalf("bind valid = %v; want %v (HTTP %d)", got, tc.valid, recorder.Code)
			}
			if tc.valid {
				return
			}
			var body struct {
				Error     struct{ Code string }
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 400 || body.Error.Code != "VALIDATION_ERROR" || body.RequestID != "bind-test" || !c.IsAborted() {
				t.Fatalf("invalid request must abort with standard 400 envelope: %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
