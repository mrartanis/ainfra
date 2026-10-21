package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFeedReassessesInventoryAndRetainsOnOutage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	ad := domain.Principal{Role: "admin"}
	n, _ := s.SaveNode(ctx, ad, domain.Node{Name: "debian"}, 0)
	p := domain.Principal{Role: "agent", Instance: "i", Nodes: []string{n.ID}}
	data, _ := json.Marshal(domain.PackageData{Packages: []domain.Package{{Name: "libssl", Version: "9", Source: "openssl", SourceVersion: "1.0-1", Origin: "Debian", OriginEvidence: "apt-policy", Suite: "bookworm", Candidate: "9.1", CandidateSource: "openssl", CandidateSourceVersion: "1.0-2"}}})
	r := domain.Report{SchemaVersion: 1, ID: "r", NodeID: n.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"id":"debian","codename":"bookworm"}`)}, "packages": {Status: "ok", Complete: true, ObservedAt: now, Data: data}}}
	if _, e := s.IngestReport(ctx, p, r); e != nil {
		t.Fatal(e)
	}
	now = now.Add(time.Hour)
	a := domain.Advisory{ID: "CVE-test", Provider: "debian", Distro: "debian", Release: "bookworm", Source: "openssl", Fixed: "1.0-2", Status: "resolved"}
	if e := s.SyncFeed(ctx, "debian", []domain.Advisory{a}, "etag", "", nil); e != nil {
		t.Fatal(e)
	}
	fs, e := s.Findings(ctx, ad, n.ID)
	if e != nil || len(fs) != 1 || fs[0].Assessment != "affected" || fs[0].Remediation != "candidate_fixes" || !fs[0].InventoryAt.Equal(r.ObservedAt) {
		t.Fatalf("%+v %v", fs, e)
	}
	if e = s.SyncFeed(ctx, "debian", nil, "", "", nil); e == nil {
		t.Fatal("empty accepted")
	}
	fs, _ = s.Findings(ctx, ad, n.ID)
	if fs[0].Workflow == "resolved" {
		t.Fatal("outage resolved finding")
	}
}
func TestUbuntuUnfixedAndUnknownOrigin(t *testing.T) {
	a := domain.Advisory{Distro: "ubuntu", Source: "foo", Release: "24.04:LTS", Status: "unknown"}
	p := domain.Package{Source: "foo", SourceVersion: "1", Origin: "Ubuntu", OriginEvidence: "apt-policy"}
	assessment, remediation := app.Assess(p, a)
	if assessment != "affected_or_under_investigation" || remediation != "unknown" {
		t.Fatal(assessment, remediation)
	}
	p.Origin = ""
	assessment, _ = app.Assess(p, a)
	if assessment != "unknown" {
		t.Fatal(assessment)
	}
}

func TestAttentionSuppressionExpiresWithoutNewReport(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	store := testkit.NewStore()
	s := app.New(store, func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, _ := s.SaveNode(ctx, admin, domain.Node{Name: "node"}, 0)
	f := domain.Finding{ID: "f", NodeID: n.ID, Assessment: "affected", Remediation: "no_fix_published", Workflow: "open"}
	if e := store.Write(ctx, func(tx app.Tx) error { return tx.Put("finding", f.ID, f) }); e != nil {
		t.Fatal(e)
	}
	if e := s.Suppress(ctx, admin, f.ID, "maintenance window", now.Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	before, e := s.Attention(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(before)
	if strings.Contains(string(raw), "no_fix_published") {
		t.Fatal(string(raw))
	}
	now = now.Add(2 * time.Hour)
	after, e := s.Attention(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(after)
	if !strings.Contains(string(raw), "no_fix_published") {
		t.Fatal("suppression expiry hidden", string(raw))
	}
}
