package routing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTiandituShortestRoute(t *testing.T) {
	p := NewTianditu("secret-key")
	p.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.tianditu.gov.cn" || r.URL.Path != "/drive" {
			t.Fatal("wrong provider endpoint")
		}
		var params map[string]string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("postStr")), &params); err != nil {
			t.Fatal(err)
		}
		if params["style"] != "1" || params["orig"] != "119.65,29.08" || params["dest"] != "119.82,28.9" {
			t.Fatal(params)
		}
		if r.URL.Query().Get("tk") != "secret-key" {
			t.Fatal("key missing")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<result><distance>33.6</distance><duration>2864</duration></result>`))}, nil
	})}
	meters, err := p.Distance(context.Background(), Point{119.65, 29.08}, Point{119.82, 28.9})
	if err != nil || meters != 33600 {
		t.Fatalf("distance %d: %v", meters, err)
	}
}

func TestTiandituRejectsInvalidAndRedactsCredentials(t *testing.T) {
	for _, body := range []string{`<result/>`, `<result><distance>-1</distance></result>`, `<result><distance>NaN</distance></result>`, `<result><distance>Inf</distance></result>`, `{"code":301012,"msg":"secret-key"}`, `not xml`} {
		t.Run(body, func(t *testing.T) {
			p := NewTianditu("secret-key")
			p.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			if _, err := p.Distance(context.Background(), Point{119, 29}, Point{120, 29}); err == nil || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("invalid provider response error: %v", err)
			}
		})
	}
	p := NewTianditu("secret-key")
	p.client = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("https://api.tianditu.gov.cn?tk=secret-key")
	})}
	if _, err := p.Distance(context.Background(), Point{}, Point{}); err == nil || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("transport error leaked: %v", err)
	}
}

func TestNextRefreshShanghai(t *testing.T) {
	for _, tc := range []struct{ now, want string }{{"2026-11-11T17:59:00Z", "2026-11-11T18:00:00Z"}, {"2026-11-11T18:00:00Z", "2026-11-12T18:00:00Z"}, {"2026-12-31T20:00:00Z", "2027-01-01T18:00:00Z"}} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		if got := nextRefresh(now, 2).UTC().Format(time.RFC3339); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.now, got, tc.want)
		}
	}
}
