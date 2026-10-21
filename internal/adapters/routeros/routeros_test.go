package routeros_test

import (
	"ainfra/internal/adapters/routeros"
	"context"
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
