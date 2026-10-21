package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIngestOwnershipOrderingAndPartial(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin", ID: "human"}
	n, err := svc.SaveNode(ctx, admin, domain.Node{Name: "host", Purpose: "manual purpose", Criticality: "high"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "instance"}
	r := domain.Report{SchemaVersion: 1, ID: "report1", NodeID: n.ID, Instance: "instance", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"packages": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"packages":[{"name":"a","version":"1"}]}`)}}}
	if _, err = svc.IngestReport(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	if ack, err := svc.IngestReport(ctx, p, r); err != nil || ack.Status != "duplicate" {
		t.Fatalf("duplicate: %+v %v", ack, err)
	}
	r.Collectors["packages"] = domain.Section{Status: "error", ErrorCode: "permission_denied"}
	if _, err = svc.IngestReport(ctx, p, r); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("conflicting ID: %v", err)
	}
	r.ID = "report2"
	r.Sequence = 2
	if _, err = svc.IngestReport(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Node(ctx, admin, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Purpose != "manual purpose" || got.Criticality != "high" || string(got.Observations["packages"].Data) != `{"packages":[{"name":"a","version":"1"}]}` {
		t.Fatalf("lost facts: %+v", got)
	}
	if got.CollectorStatus["packages"].Status != "error" {
		t.Fatal("error hidden")
	}
	r.ID = "report-old"
	r.Sequence = 1
	r.Collectors = map[string]domain.Section{}
	if ack, err := svc.IngestReport(ctx, p, r); err != nil || ack.Status != "out_of_order" {
		t.Fatalf("order: %+v %v", ack, err)
	}
	r.ID = "report3"
	r.Sequence = 3
	r.Collectors["packages"] = domain.Section{Status: "partial", ObservedAt: now, Data: json.RawMessage(`{"packages":[]}`)}
	if _, err = svc.IngestReport(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Node(ctx, admin, n.ID)
	if string(got.Observations["packages"].Data) == `{"packages":[]}` {
		t.Fatal("partial cleared packages")
	}
	r.ID = "report4"
	r.Sequence = 4
	r.Collectors["packages"] = domain.Section{Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"packages":[]}`)}
	if _, err = svc.IngestReport(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Node(ctx, admin, n.ID)
	if string(got.Observations["packages"].Data) != `{"packages":[]}` {
		t.Fatal("complete did not clear")
	}
	if _, err = svc.Node(ctx, p, n.ID); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("agent read allowed")
	}
	p.Nodes = []string{"other"}
	r.ID = "report5"
	if _, err = svc.IngestReport(ctx, p, r); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("cross-node write allowed")
	}
}

func TestReportRejectsUnsafeOrUnboundedSectionPayload(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	n, err := svc.SaveNode(ctx, domain.Principal{Role: "admin"}, domain.Node{Name: "node"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.Principal{Role: "agent", Instance: "agent", Nodes: []string{n.ID}}
	cases := []struct {
		name string
		data string
	}{
		{"os", `{"id":"debian","password":"private"}`},
		{"packages", `{"packages":[{"name":"pkg","version":"1","private_key":"private"}]}`},
		{"wireguard", `{"private-key":"private"}`},
		{"network", `{"routes":[{"nested":{"preshared_key":"private"}}]}`},
		{"network", `"not an observation object"`},
		{"network", `{"nested":` + strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20) + "}"},
		{"wireguard", `{"peers":"` + strings.Repeat("x", (1<<20)+1) + `"}`},
	}
	for _, test := range cases {
		t.Run(test.name+test.data[:5], func(t *testing.T) {
			r := domain.Report{SchemaVersion: 1, ID: "r", NodeID: n.ID, Instance: "agent", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{test.name: {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(test.data)}}}
			if _, err := svc.IngestReport(ctx, principal, r); !errors.Is(err, app.ErrInvalid) {
				t.Fatal("invalid payload accepted", err)
			}
		})
	}
}
