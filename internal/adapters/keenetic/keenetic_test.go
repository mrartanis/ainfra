package keenetic

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthenticationNormalizationAndSecretExclusion(t *testing.T) {
	own := base64.StdEncoding.EncodeToString(make([]byte, 32))
	peer := base64.StdEncoding.EncodeToString(bytesRepeat(1, 32))
	logins := 0
	expired := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth" {
			if r.Method == "GET" {
				w.Header().Set("X-NDM-Realm", "model-specific realm")
				w.Header().Set("X-NDM-Challenge", "challenge")
				http.SetCookie(w, &http.Cookie{Name: fmt.Sprintf("session%d", logins), Value: "challenge-cookie", Path: "/"})
				w.WriteHeader(401)
				return
			}
			if r.Method != "POST" {
				t.Error("unexpected auth method")
			}
			var auth map[string]string
			json.NewDecoder(r.Body).Decode(&auth)
			first := md5.Sum([]byte("reader:model-specific realm:password"))
			proof := sha256.Sum256([]byte("challenge" + hex.EncodeToString(first[:])))
			cookie, err := r.Cookie(fmt.Sprintf("session%d", logins))
			if auth["login"] != "reader" || auth["password"] != hex.EncodeToString(proof[:]) || err != nil || cookie.Value != "challenge-cookie" {
				t.Error("invalid challenge response or cookie")
			}
			logins++
			http.SetCookie(w, &http.Cookie{Name: "authenticated", Value: fmt.Sprint(logins), Path: "/"})
			return
		}
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/rci/show/") {
			t.Fatal("modifying or configuration request")
		}
		cookie, err := r.Cookie("authenticated")
		if err != nil || cookie.Value != fmt.Sprint(logins) {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/rci/show/system" && !expired {
			expired = true
			logins++
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/rci/show/version":
			fmt.Fprint(w, `{"title":"5.1.6","arch":"mips","model":"Test","private-key":"NEVER_EXPORT"}`)
		case "/rci/show/system":
			fmt.Fprint(w, `{"hostname":"router","memtotal":131072,"memfree":1000,"uptime":"120","cpuload":5,"password":"NEVER_EXPORT"}`)
		case "/rci/show/interface":
			fmt.Fprintf(w, `{"0":{"id":"GigabitEthernet0/0","type":"Port","link":"up"},"Wireguard0":{"id":"Wireguard0","type":"Wireguard","state":"up","wireguard":{"public-key":%q,"private-key":"NEVER_EXPORT","peer":[{"public-key":%q,"enabled":true,"last-handshake":2147483647,"preshared-key":"NEVER_EXPORT"}]}}}`, own, peer)
		case "/rci/show/ip/route":
			fmt.Fprint(w, `[{"destination":"0.0.0.0/0","gateway":"192.0.2.1","interface":"WAN","metric":10,"secret":"NEVER_EXPORT"}]`)
		case "/rci/show/ipv6/route":
			fmt.Fprint(w, `{"route6":[]}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	}))
	defer server.Close()
	now := time.Unix(2000000000, 0)
	result := New(server.Client(), server.URL, "reader", "password", func() time.Time { return now }).Collect(context.Background(), nil)
	if logins != 3 {
		t.Fatal("expected initial auth and one re-auth", logins)
	}
	for name, section := range result {
		if section.Status != "ok" || !section.Complete || strings.Contains(string(section.Data), "NEVER_EXPORT") {
			t.Fatal(name, section.Status, string(section.Data))
		}
	}
	var os map[string]any
	json.Unmarshal(result["os"].Data, &os)
	if os["memory_bytes"] != float64(134217728) || os["uptime_seconds"] != float64(120) || os["cpu"] != nil {
		t.Fatal(os)
	}
	var wg map[string]any
	json.Unmarshal(result["wireguard"].Data, &wg)
	if wg["latest-handshakes"] != "" || !strings.Contains(wg["peers"].(string), peer) {
		t.Fatal(wg)
	}
}

func bytesRepeat(v byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func TestFailuresDoNotBecomeSuccessfulEmptySnapshots(t *testing.T) {
	for _, body := range []string{`{"status":[{"status":"error","code":"1","message":"DO_NOT_EXPORT"}]}`, `null`, `not json`, strings.Repeat(" ", 2<<20) + "{}"} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer s.Close()
			result := New(s.Client(), s.URL, "reader", "password", time.Now).Collect(context.Background(), []string{"os"})
			if result["os"].Status != "error" || len(result["os"].Data) != 0 {
				t.Fatal(result)
			}
		})
	}
}

func TestTLSAndRedirectBoundaries(t *testing.T) {
	hits := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	for _, test := range []struct {
		client *http.Client
		base   string
	}{{source.Client(), source.URL}, {http.DefaultClient, source.URL}, {source.Client(), "http://127.0.0.1"}} {
		result := New(test.client, test.base, "reader", "password", time.Now).Collect(context.Background(), []string{"os"})
		if result["os"].Status != "error" {
			t.Fatal(result)
		}
	}
	if hits != 0 {
		t.Fatal("followed redirect", hits)
	}
}

func TestRejectedCredentialsAreNotRetriedByEverySection(t *testing.T) {
	logins := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth" && r.Method == "POST" {
			logins++
		}
		w.Header().Set("X-NDM-Realm", "test")
		w.Header().Set("X-NDM-Challenge", "challenge")
		w.WriteHeader(401)
	}))
	defer s.Close()
	result := New(s.Client(), s.URL, "reader", "invalid", time.Now).Collect(context.Background(), nil)
	if logins != 1 || result["os"].Status != "error" || result["network"].Status != "error" {
		t.Fatal(logins, result)
	}
}

func TestRouteShapesAndPartialMetrics(t *testing.T) {
	for _, raw := range []string{`{}`, `{"route6":[]}`, `[]`} {
		if rows, err := parseRoutes([]byte(raw)); err != nil || len(rows) != 0 {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`null`, `{"other":[]}`, `[{"destination":"invalid"}]`} {
		if _, err := parseRoutes([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "version") {
			fmt.Fprint(w, `{"title":"5.1.6"}`)
		} else {
			fmt.Fprint(w, `{"memtotal":"invalid","cpuload":101,"uptime":"invalid"}`)
		}
	}))
	defer s.Close()
	section := New(s.Client(), s.URL, "reader", "password", time.Now).Collect(context.Background(), []string{"os"})["os"]
	if section.Status != "partial" || strings.Contains(string(section.Data), "memory_bytes") || strings.Contains(string(section.Data), "uptime_seconds") {
		t.Fatal(section)
	}
}
