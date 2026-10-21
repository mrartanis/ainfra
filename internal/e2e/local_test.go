package e2e_test

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/agent"
	"ainfra/internal/apiclient"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type localCollector struct{ now time.Time }

func (c localCollector) Collect(context.Context, []string) map[string]domain.Section {
	return map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: c.now, Data: json.RawMessage(`{"id":"debian","codename":"bookworm"}`)}, "packages": {Status: "ok", Complete: true, ObservedAt: c.now, Data: json.RawMessage(`{"packages":[{"name":"libssl3","arch":"amd64","version":"3.0.1","source":"openssl","source_version":"3.0.1","state":"installed","origin":"Debian","origin_evidence":"apt-policy","suite":"bookworm","candidate":"3.0.2","candidate_source":"openssl","candidate_source_version":"3.0.2"}]}`)}}
}
func TestAgentAPIPlanBackupRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, e := sqlite.Open(filepath.Join(dir, "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	svc := app.New(store, time.Now, app.RandomID)
	local := domain.Principal{ID: "human", Role: "admin"}
	admin, _, e := svc.IssueToken(ctx, local, domain.Principal{Role: "admin"})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(httpapi.New(svc))
	defer server.Close()
	c := &apiclient.Client{Base: server.URL, Token: admin, HTTP: server.Client()}
	var n domain.Node
	if e = c.Do(ctx, "POST", "nodes", domain.Node{Name: "test", Purpose: "manual"}, &n, 0); e != nil {
		t.Fatal(e)
	}
	token, _, e := svc.IssueToken(ctx, local, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "instance"})
	if e != nil {
		t.Fatal(e)
	}
	sender := agent.APITransport{Client: &apiclient.Client{Base: server.URL, Token: token, HTTP: server.Client()}}
	a := agent.New(agent.Config{NodeID: n.ID, Instance: "instance", StateDir: filepath.Join(dir, "agent")}, localCollector{time.Now()}, sender, time.Now, app.RandomID)
	if e = a.Run(ctx); e != nil {
		t.Fatal(e)
	}
	if e = c.Do(ctx, "GET", "nodes/"+n.ID, nil, &n, 0); e != nil || n.Sequence != 1 || n.Purpose != "manual" {
		t.Fatal(n, e)
	}
	if e = svc.SyncFeed(ctx, "debian", []domain.Advisory{{ID: "CVE-fixture", Distro: "debian", Provider: "debian", Release: "bookworm", Source: "openssl", Fixed: "3.0.2", Status: "resolved"}}, "", "", nil); e != nil {
		t.Fatal(e)
	}
	var findings []domain.Finding
	if e = c.Do(ctx, "GET", "nodes/"+n.ID+"/findings", nil, &findings, 0); e != nil || len(findings) != 1 || findings[0].Remediation != "candidate_fixes" {
		t.Fatal(findings, e)
	}
	editor, _, _ := svc.IssueToken(ctx, local, domain.Principal{Role: "editor"})
	ed := *c
	ed.Token = editor
	var doc domain.Document
	if e = ed.Do(ctx, "POST", "documents", domain.Document{Title: "Plan", Kind: "maintenance_plan", Body: "Verify backup first", Nodes: []string{n.ID}}, &doc, 0); e != nil {
		t.Fatal(e)
	}
	if e = ed.Do(ctx, "POST", "documents/"+doc.ID+"/approve", nil, nil, doc.Revision); e == nil {
		t.Fatal("editor approved plan")
	}
	if e = c.Do(ctx, "POST", "documents/"+doc.ID+"/approve", nil, &doc, doc.Revision); e != nil || doc.Status != "approved" {
		t.Fatal(doc, e)
	}
	backup := filepath.Join(dir, "backup.db")
	if e = store.Backup(ctx, backup); e != nil {
		t.Fatal(e)
	}
	restored, e := sqlite.Open(backup)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	rs := app.New(restored, time.Now, app.RandomID)
	principal, e := rs.Authenticate(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	got, e := rs.Document(ctx, principal, doc.ID, 0)
	if e != nil || got.ApprovedHash != doc.ApprovedHash {
		t.Fatal(got, e)
	}
	hits, e := rs.Search(ctx, principal, "Verify", false, 20, 0)
	if e != nil || len(hits) != 1 {
		t.Fatal(hits, e)
	}
}
