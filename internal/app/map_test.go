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

func TestPublicServicesContextIncludesNestedGuestsAndIgnoresPrivateServices(t *testing.T) {
	svc := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	host, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "physical"}, 0)
	vm, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "vm"}, 0)
	guest, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "plex", Services: []domain.Service{{Name: "Jellyfin", Exposure: "public", Primary: true}, {Name: "InternalAdmin", Exposure: "private"}}}, 0)
	for _, r := range []domain.Relation{{Source: vm.ID, Target: host.ID, Type: "runs_on"}, {Source: guest.ID, Target: vm.ID, Type: "runs_on"}} {
		if _, err := svc.SaveRelation(ctx, admin, r); err != nil {
			t.Fatal(err)
		}
	}
	text, err := svc.Context(ctx, admin, host.ID)
	if err != nil || !strings.Contains(text, "Jellyfin") || !strings.Contains(text, "Internet-facing services") || strings.Contains(text, "InternalAdmin") {
		t.Fatal(text, err)
	}
	topology, err := svc.Topology(ctx, domain.Principal{Role: "reader"})
	if err != nil || len(topology.Nodes) != 3 || len(topology.Relations) != 2 {
		t.Fatal(topology, err)
	}
	if _, err = svc.Topology(ctx, domain.Principal{Role: "agent"}); err != app.ErrForbidden {
		t.Fatal(err)
	}
}

func TestAPTUnknownIsNotAttentionButManagedRefreshFailureIs(t *testing.T) {
	for _, status := range []string{"", "error"} {
		now := time.Now().UTC()
		svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
		ctx := context.Background()
		admin := domain.Principal{Role: "admin"}
		n, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "node"}, 0)
		pd := domain.PackageData{MetadataStatus: "unknown", Packages: []domain.Package{
			{Name: "2048-qt", Version: "0.1.6-2build1", Candidate: "0.1.6-2build1"},
			{Name: "accountsservice", Version: "22.07.5-2ubuntu1.6", Candidate: "22.07.5-2ubuntu1.6"},
			{Name: "upgrade", Version: "1", Candidate: "2"},
			{Name: "older", Version: "2", Candidate: "1"},
		}}
		if status != "" {
			pd.MetadataRefresh = &domain.APTRefresh{Status: status, LastAttempt: now}
		}
		raw, _ := json.Marshal(pd)
		if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}, domain.Report{SchemaVersion: 1, ID: "r", NodeID: n.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"packages": {Status: "ok", Complete: true, ObservedAt: now, Data: raw}}}); err != nil {
			t.Fatal(err)
		}
		updates, err := svc.Packages(ctx, admin, n.ID, true)
		if err != nil || len(updates.Packages) != 1 || updates.Packages[0].Name != "upgrade" {
			t.Fatal(updates, err)
		}
		attention, err := svc.Attention(ctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		apt := false
		for _, reason := range attention[0].Reasons {
			if strings.HasPrefix(reason, "APT:") {
				apt = true
			}
		}
		if apt != (status == "error") {
			t.Fatal(status, attention)
		}
	}
}
