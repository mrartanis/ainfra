package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestZoomAssetAndScopedContentSecurityPolicy(t *testing.T) {
	u := New("http://unused", nil, false, time.Now)
	for _, tc := range []struct {
		path, method string
		status       int
		script       bool
	}{
		{"/map.js", "GET", 200, true}, {"/map.js", "HEAD", 200, true}, {"/map.js", "POST", 405, true}, {"/map", "GET", 303, true}, {"/login", "GET", 200, false},
	} {
		w := httptest.NewRecorder()
		u.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code)
		}
		policy := w.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "script-src 'self'") != tc.script || strings.Contains(policy, "unsafe-inline") || strings.Contains(policy, "unsafe-eval") {
			t.Fatal(tc, policy)
		}
		if tc.path == "/map.js" && tc.method == "GET" {
			if !strings.Contains(w.Header().Get("Content-Type"), "application/javascript") || !strings.Contains(w.Body.String(), "data-map-viewer") || w.Header().Get("Set-Cookie") != "" {
				t.Fatal(w.Header(), w.Body.String())
			}
		}
		if tc.path == "/map.js" && tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD returned script body")
		}
	}
}
