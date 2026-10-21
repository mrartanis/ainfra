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
	data, _ := json.Marshal(domain.PackageData{Packages: []domain.Package{{Name: "libssl", Version: "9", Source: "openssl", SourceVersion: "1.0-1", Origin: "Debian", OriginEvidence: "apt-policy", Suite: "bookworm", Candidate: "9.1", CandidateOrigin: "Debian", CandidateSuite: "bookworm", CandidateOriginEvidence: "apt-policy", CandidateSource: "openssl", CandidateSourceVersion: "1.0-2"}}})
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

func TestRepeatedAssessmentPreservesClosureTime(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, err := s.SaveNode(ctx, admin, domain.Node{Name: "node"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{Role: "agent", Instance: "agent", Nodes: []string{n.ID}}
	a := domain.Advisory{ID: "CVE-test", Provider: "debian", Distro: "debian", Release: "bookworm", Source: "openssl", Fixed: "2", Status: "resolved"}
	if err = s.SyncFeed(ctx, "debian", []domain.Advisory{a}, "", "", nil); err != nil {
		t.Fatal(err)
	}
	report := func(seq uint64, version string, absent bool) {
		t.Helper()
		pkgs := []domain.Package{{Name: "ssl", Source: "openssl", SourceVersion: version, Version: version, Origin: "Debian", OriginEvidence: "apt-policy", Suite: "bookworm"}}
		if absent {
			pkgs = nil
		}
		data, _ := json.Marshal(domain.PackageData{Packages: pkgs})
		r := domain.Report{SchemaVersion: 1, ID: now.String(), NodeID: n.ID, Instance: "agent", Sequence: seq, ObservedAt: now, Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"id":"debian","codename":"bookworm"}`)}, "packages": {Status: "ok", Complete: true, ObservedAt: now, Data: data}}}
		if _, err := s.IngestReport(ctx, p, r); err != nil {
			t.Fatal(err)
		}
	}
	report(1, "1", false)
	now = now.Add(time.Hour)
	report(2, "2", false)
	closed := now
	for i := 0; i < 2; i++ {
		now = now.Add(24 * time.Hour)
		report(uint64(i+3), "2", i == 1)
		fs, err := s.Findings(ctx, admin, n.ID)
		if err != nil || len(fs) != 1 {
			t.Fatal(fs, err)
		}
		raw, _ := json.Marshal(fs[0])
		var wire map[string]any
		json.Unmarshal(raw, &wire)
		if wire["resolved_at"] != closed.Format(time.RFC3339) {
			t.Fatalf("closure changed or missing: %s", raw)
		}
	}
	now = now.Add(time.Hour)
	report(5, "1", false)
	fs, _ := s.Findings(ctx, admin, n.ID)
	if fs[0].Workflow != "open" {
		t.Fatal(fs)
	}
	raw, _ := json.Marshal(fs[0])
	var wire map[string]any
	json.Unmarshal(raw, &wire)
	if v := wire["resolved_at"]; v != nil && v != "0001-01-01T00:00:00Z" {
		t.Fatalf("reopened finding retains closure: %s", raw)
	}
}

func TestMixedReleasePackageIsNotConfidentlyAssessed(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, err := s.SaveNode(ctx, admin, domain.Node{Name: "mixed"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Advisory{ID: "CVE-test", Provider: "debian", Distro: "debian", Release: "bookworm", Source: "openssl", Fixed: "2", Status: "resolved"}
	if err = s.SyncFeed(ctx, "debian", []domain.Advisory{a}, "", "", nil); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(domain.PackageData{Packages: []domain.Package{{Name: "ssl", Source: "openssl", SourceVersion: "1", Version: "1", Origin: "Debian", OriginEvidence: "apt-policy", Suite: "trixie"}}})
	r := domain.Report{SchemaVersion: 1, ID: "report", NodeID: n.ID, Instance: "agent", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"id":"debian","codename":"bookworm"}`)}, "packages": {Status: "ok", Complete: true, ObservedAt: now, Data: data}}}
	if _, err = s.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "agent"}, r); err != nil {
		t.Fatal(err)
	}
	fs, err := s.Findings(ctx, admin, n.ID)
	if err != nil || len(fs) != 1 || fs[0].Assessment != "unknown" {
		t.Fatal(fs, err)
	}
}

func TestCandidateNeedsMatchingVendorAndReleaseEvidence(t *testing.T) {
	a := domain.Advisory{Distro: "ubuntu", Source: "foo", Fixed: "2", Release: "24.04:LTS", Channel: "Pro"}
	p := domain.Package{Source: "foo", SourceVersion: "1", Version: "1", Origin: "Ubuntu", OriginEvidence: "apt-policy", Suite: "noble", CandidateSource: "foo", CandidateSourceVersion: "3"}
	for _, test := range []struct{ origin, suite, evidence, want string }{
		{"", "", "", "fix_published_not_candidate"},
		{"LP-PPA-foo", "noble", "apt-policy", "fix_published_not_candidate"},
		{"UbuntuESMApps", "jammy", "apt-policy", "fix_published_not_candidate"},
		{"UbuntuESMApps", "noble-apps-security", "apt-policy", "candidate_fixes"},
	} {
		p.CandidateOrigin = test.origin
		p.CandidateSuite = test.suite
		p.CandidateOriginEvidence = test.evidence
		as, rem := app.Assess(p, a)
		if as != "affected" || rem != test.want {
			t.Fatal(test, as, rem)
		}
	}
}
