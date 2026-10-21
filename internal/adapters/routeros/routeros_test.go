package routeros_test

import (
	"ainfra/internal/adapters/routeros"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTLSGetProplistOnly(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Query().Get(".proplist") == "" {
			t.Error("unsafe request")
		}
		if strings.Contains(r.URL.RawQuery, "private") || strings.Contains(r.URL.RawQuery, "preshared") {
			t.Error("secret requested")
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "readonly" || p != "test" {
			t.Error("auth")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/system/resource" {
			w.Write([]byte(`[{"version":"7.20","cpu-count":"2","total-memory":"1000","free-memory":"500","architecture-name":"arm64"}]`))
		} else {
			w.Write([]byte(`[]`))
		}
	}))
	defer s.Close()
	c := routeros.New(s.Client(), s.URL, "readonly", "test", time.Now)
	out := c.Collect(context.Background(), nil)
	if out["os"].Status != "ok" || calls < 4 {
		t.Fatal(out, calls)
	}
}

func TestResourceMetricsPreserveUnknownAndParseUptime(t *testing.T) {
	for _, test := range []struct {
		uptime string
		want   float64
	}{{"1w2d3h4m5s", 788645}, {"2d03:04:05", 183845}, {"invalid", -1}} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/rest/system/resource" {
				json.NewEncoder(w).Encode([]map[string]string{{"version": "7.20", "cpu-count": "2", "cpu-load": "7", "total-memory": "1000", "free-memory": "500", "total-hdd-space": "10000", "free-hdd-space": "8000", "board-name": "test-board", "uptime": test.uptime}})
			} else {
				w.Write([]byte(`[]`))
			}
		}))
		result := routeros.New(server.Client(), server.URL, "user", "test", time.Now).Collect(context.Background(), nil)
		server.Close()
		var value map[string]any
		if err := json.Unmarshal(result["os"].Data, &value); err != nil {
			t.Fatal(err)
		}
		if value["memory_free_bytes"] != float64(500) || value["storage_free_bytes"] != float64(8000) || value["model"] != "test-board" {
			t.Fatal(value)
		}
		if test.want >= 0 && value["uptime_seconds"] != test.want {
			t.Fatal(value)
		}
		if test.want < 0 && (value["uptime_seconds"] != nil || result["os"].Status != "partial") {
			t.Fatal("invalid uptime presented as known", value)
		}
	}
}
