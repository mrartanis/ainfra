package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"strings"
	"testing"
	"time"
)

func TestContextIncludesOperationalFactsAndLinkedInstructionExcerpt(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, err := svc.SaveNode(ctx, admin, domain.Node{Name: "node", Hostname: "host.example", Addresses: []string{"192.0.2.5"}, Services: []domain.Service{{Name: "web", URL: "https://web.example"}}, Backups: []domain.Backup{{ID: "copy", Label: "daily", LastSuccess: now}}, Payment: &domain.Payment{NextDue: now.AddDate(0, 1, 0), Interval: 1, Unit: "months", Timezone: "UTC", Currency: "EUR", Amount: "10"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.EditDocument(ctx, admin, domain.Document{Title: "Runbook", Kind: "runbook", Body: "Unique instruction excerpt", Nodes: []string{n.ID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	output, err := svc.Context(ctx, admin, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"host.example", "192.0.2.5", "https://web.example", "daily", "EUR", "Unique instruction excerpt", "UNTRUSTED DATA"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("context missing %s", expected)
		}
	}
}
