package main

import (
	"ainfra/internal/agent"
	"ainfra/internal/domain"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeContinuesAfterTargetFailureAndSendsKeeneticSections(t *testing.T) {
	dir := t.TempDir()
	received := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rci/show/version":
			fmt.Fprint(w, `{"title":"5.1.6","model":"test"}`)
		case "/rci/show/system":
			fmt.Fprint(w, `{"uptime":"10"}`)
		case "/rci/show/interface":
			fmt.Fprint(w, `{}`)
		case "/rci/show/ip/route", "/rci/show/ipv6/route":
			fmt.Fprint(w, `[]`)
		case "/api/v1/ingest/reports":
			var report domain.Report
			if json.NewDecoder(r.Body).Decode(&report) != nil || report.NodeID != "good" || report.Instance != "instance" {
				t.Error("wrong target identity")
			}
			if len(report.Collectors) != 3 || report.Collectors["wireguard"].Status != "ok" {
				t.Error("missing probe sections")
			}
			received = true
			fmt.Fprint(w, `{"status":"queued"}`)
		default:
			t.Error("unexpected request", r.URL.Path)
		}
	}))
	defer server.Close()
	ca := filepath.Join(dir, "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	token := filepath.Join(dir, "token")
	os.WriteFile(token, []byte("token"), 0600)
	credentials := filepath.Join(dir, "credentials")
	os.WriteFile(credentials, []byte(`{"username":"reader","password":"test"}`), 0600)
	config := agent.Config{NodeID: "probe", Instance: "instance", Server: server.URL, CAFile: ca, StateDir: dir, TokenFile: token, ProbeTargets: []agent.ProbeTarget{
		{Type: "keenetic", NodeID: "bad", URL: server.URL, CAFile: ca, CredentialFile: filepath.Join(dir, "missing"), TokenFile: token},
		{Type: "keenetic", NodeID: "good", URL: server.URL, CAFile: ca, CredentialFile: credentials, TokenFile: token},
	}}
	raw, _ := json.Marshal(config)
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, raw, 0600)
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"infra-agent", "probe", "--config", path}
	if err := run(); err == nil {
		t.Fatal("missing target failure")
	}
	if !received {
		t.Fatal("failed target prevented next target")
	}
}
