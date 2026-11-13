package routing

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Point struct{ Longitude, Latitude float64 }

// Provider is replaceable so tests never consume live routing quotas.
type Provider interface {
	Distance(context.Context, Point, Point) (int64, error)
}

type Tianditu struct {
	key    string
	client *http.Client
}

func NewTianditu(key string) *Tianditu {
	return &Tianditu{key: strings.TrimSpace(key), client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var errRouteUnavailable = errors.New("route unavailable")
var errProviderDenied = errors.New("route provider authorization or quota denied")

func (p *Tianditu) Distance(ctx context.Context, from, to Point) (int64, error) {
	if p.key == "" {
		return 0, errProviderDenied
	}
	coord := func(point Point) string {
		return strconv.FormatFloat(point.Longitude, 'f', -1, 64) + "," + strconv.FormatFloat(point.Latitude, 'f', -1, 64)
	}
	params, _ := json.Marshal(map[string]string{"orig": coord(from), "dest": coord(to), "style": "1"})
	query := url.Values{"postStr": {string(params)}, "type": {"search"}, "tk": {p.key}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.tianditu.gov.cn/drive?"+query.Encode(), nil)
	if err != nil {
		return 0, errRouteUnavailable
	}
	response, err := p.client.Do(req)
	// Transport errors contain the request URL (and tk). Never propagate them.
	if err != nil {
		return 0, errRouteUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 429 {
		return 0, errProviderDenied
	}
	if response.StatusCode != 200 {
		return 0, errRouteUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return 0, errRouteUnavailable
	}
	var problem struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(raw, &problem) == nil && problem.Code != 0 {
		return 0, errProviderDenied
	}
	var result struct {
		XMLName  xml.Name `xml:"result"`
		Distance *float64 `xml:"distance"`
	}
	if xml.Unmarshal(raw, &result) != nil || result.Distance == nil {
		return 0, errRouteUnavailable
	}
	km := *result.Distance
	if math.IsNaN(km) || math.IsInf(km, 0) || km < 0 || km > 100000 {
		return 0, errRouteUnavailable
	}
	// Tianditu's top-level distance is km; individual streetDistance fields are m.
	return int64(math.Round(km * 1000)), nil
}
