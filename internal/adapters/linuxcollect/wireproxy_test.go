package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWireProxyCollectsOnlyPublicStateWithoutCommands(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	peer := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" || r.Method != "GET" {
			t.Error(r.Method, r.URL)
		}
		fmt.Fprintf(w, "private_key=REDACTED\nlisten_port=51821\npublic_key=%x\npreshared_key=REDACTED\nendpoint=192.0.2.1:51820\nlast_handshake_time_sec=123\nallowed_ip=0.0.0.0/0\nallowed_ip=::/0\nrx_bytes=100\n", []byte(strings.Repeat("b", 32)))
	}))
	defer server.Close()
	runner := &fakeRunner{}
	now := time.Now()
	c := linuxcollect.New(runner, os.ReadFile, func() time.Time { return now }).WithWireProxy(server.URL+"/metrics", key)
	section := c.WireGuard(context.Background())
	if section.Status != "ok" || !section.Complete || !section.ObservedAt.Equal(now) || len(runner.calls) > 0 {
		t.Fatal(section, runner.calls)
	}
	var fields map[string]string
	if err := json.Unmarshal(section.Data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 5 || fields["public-key"] != "wireproxy\t"+key+"\n" || fields["peers"] != "wireproxy\t"+peer+"\n" || !strings.Contains(fields["latest-handshakes"], "\t123\n") || !strings.Contains(fields["allowed-ips"], "::/0") {
		t.Fatal(fields)
	}
	if strings.Contains(string(section.Data), "private") || strings.Contains(string(section.Data), "preshared") || strings.Contains(string(section.Data), "REDACTED") {
		t.Fatal("non-public fields exported", section)
	}
}

func TestWireProxyFailuresDoNotBecomeEmptyOrLeakResponse(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, body := range []string{"", "garbage", "public_key=broken\n", "private_key=DO_NOT_PUBLISH\n", "preshared_key=DO_NOT_PUBLISH\n", "endpoint=192.0.2.1:1\n", "public_key=" + strings.Repeat("11", 32) + "\nallowed_ip=bad\n", strings.Repeat("x", (1<<20)+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c := linuxcollect.New(&fakeRunner{}, os.ReadFile, time.Now).WithWireProxy(server.URL+"/metrics", key)
			s := c.WireGuard(context.Background())
			if s.Status != "error" || s.Complete || len(s.Data) > 0 {
				t.Fatal("unsafe or malformed response accepted", s)
			}
		})
	}
}

func TestWireProxyDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer redirect.Close()
	c := linuxcollect.New(&fakeRunner{}, os.ReadFile, time.Now).WithWireProxy(redirect.URL+"/metrics", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if s := c.WireGuard(context.Background()); s.Status != "error" || calls != 0 {
		t.Fatal(s, calls)
	}
}
