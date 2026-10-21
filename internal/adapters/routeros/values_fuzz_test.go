package routeros

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fuzzTransport func(*http.Request) (*http.Response, error)

func (f fuzzTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func FuzzRouterOSValues(f *testing.F) {
	for _, raw := range []string{"2", "-1", "1w2d03:04:05", "NaN", "1e309", "18446744073709551616", "", "0s"} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 4096 {
			t.Skip()
		}
		if seconds, err := parseUptime(raw); err == nil && (seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0)) {
			t.Fatal("invalid uptime accepted")
		}
		body, _ := json.Marshal([]map[string]string{{"version": "7.20", "cpu-count": raw, "total-memory": "1000", "cpu-load": raw, "free-memory": raw, "uptime": raw}})
		client := &http.Client{Transport: fuzzTransport(func(r *http.Request) (*http.Response, error) {
			data := "[]"
			if strings.HasSuffix(r.URL.Path, "system/resource") {
				data = string(body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header)}, nil
		})}
		result := New(client, "https://router.invalid", "fixture", "fixture", func() time.Time { return time.Unix(1, 0) }).Collect(context.Background(), nil)["os"]
		if len(result.Data) > 0 {
			var os domain.OSData
			if err := json.Unmarshal(result.Data, &os); err != nil {
				t.Fatal(err)
			}
			if os.CPU < 1 {
				t.Fatal("invalid CPU count accepted", os.CPU)
			}
		}
	})
}
