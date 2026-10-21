package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNodeListIsCompactButPackagesRemainAvailable(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, e := svc.SaveNode(ctx, admin, domain.Node{Name: "large"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	packages := make([]domain.Package, 5000)
	for i := range packages {
		packages[i] = domain.Package{Name: fmt.Sprintf("package-%d", i), Arch: "amd64", Version: "1", Candidate: "2", Source: "source", SourceVersion: "1"}
	}
	payload, _ := json.Marshal(domain.PackageData{Packages: packages, MetadataStatus: "unknown"})
	r := domain.Report{SchemaVersion: 1, ID: "report", NodeID: n.ID, Instance: "agent", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"packages": {Status: "ok", Complete: true, ObservedAt: now, Data: payload}}}
	if _, e = svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "agent"}, r); e != nil {
		t.Fatal(e)
	}
	nodes, e := svc.Nodes(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	wire, _ := json.Marshal(nodes)
	if len(wire) > 8192 {
		t.Fatalf("node list includes inventory: %d bytes", len(wire))
	}
	pd, e := svc.Packages(ctx, admin, n.ID, false)
	if e != nil || len(pd.Packages) != 5000 {
		t.Fatal(len(pd.Packages), e)
	}
	attention, e := svc.Attention(ctx, admin)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(attention)
	if !strings.Contains(string(b), "Обычные обновления: 5000") {
		t.Fatal(string(b))
	}
}
