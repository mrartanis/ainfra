package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBackupCheckPreservesSuccessfulCopyButExposesFailure(t *testing.T) {
	now := time.Now().UTC()
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	last := now.Add(-time.Hour)
	n, e := s.SaveNode(ctx, admin, domain.Node{Name: "n", Backups: []domain.Backup{{ID: "backup", Label: "copy", LastSuccess: last, IntervalHours: 24}}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	p := domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}
	payload := []map[string]any{{"schema_version": 1, "check_id": "backup", "kind": "backup", "observed_at": now, "status": "error", "data": map[string]any{"last_attempt_at": now, "last_attempt_status": "error"}}}
	b, _ := json.Marshal(payload)
	r := domain.Report{SchemaVersion: 1, ID: "r", NodeID: n.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"checks": {Status: "ok", Complete: true, ObservedAt: now, Data: b}}}
	if _, e = s.IngestReport(ctx, p, r); e != nil {
		t.Fatal(e)
	}
	n, e = s.Node(ctx, admin, n.ID)
	if e != nil || !n.Backups[0].LastSuccess.Equal(last) || n.Backups[0].LastAttemptStatus != "error" {
		t.Fatal(n.Backups, e)
	}
}
func TestRejectSecretCheckFields(t *testing.T) {
	if e := app.ValidateChecks(json.RawMessage(`[{"schema_version":1,"check_id":"a","kind":"backup","observed_at":"2026-10-21T00:00:00Z","status":"ok","data":{"password":"sensitive"}}]`)); e == nil {
		t.Fatal("secret accepted")
	}
}
func TestPartialChecksApplyValidResults(t *testing.T) {
	now := time.Now().UTC()
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	last := now.Add(-time.Hour)
	n, e := s.SaveNode(ctx, admin, domain.Node{Name: "n", Backups: []domain.Backup{{ID: "backup", Label: "copy", LastSuccess: last, IntervalHours: 24}}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	p := domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}
	payload := []map[string]any{{"schema_version": 1, "check_id": "backup", "kind": "backup", "observed_at": now, "status": "error", "data": map[string]any{"last_attempt_at": now, "last_attempt_status": "error"}}}
	b, _ := json.Marshal(payload)
	r := domain.Report{SchemaVersion: 1, ID: "r", NodeID: n.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"checks": {Status: "partial", Complete: false, ObservedAt: now, Data: b}}}
	if _, e = s.IngestReport(ctx, p, r); e != nil {
		t.Fatal(e)
	}
	n, e = s.Node(ctx, admin, n.ID)
	if e != nil || !n.Backups[0].LastSuccess.Equal(last) || n.Backups[0].LastAttemptStatus != "error" {
		t.Fatal(n.Backups, e)
	}
}

func TestBackupObservationFreshnessIsSeparateFromCopyFreshness(t *testing.T) {
	now := time.Now().UTC()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	n, err := svc.SaveNode(ctx, admin, domain.Node{Name: "host", Backups: []domain.Backup{{ID: "copy", Label: "daily", IntervalHours: 24, GraceHours: 2, LastSuccess: now, LastRestoreTest: now, ObservedAt: now.Add(-48 * time.Hour)}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Attention(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.NodeID == n.ID {
			for _, reason := range row.Reasons {
				if reason == "Бэкап daily: нет свежего результата проверки" {
					found = true
				}
				if reason == "Бэкап daily: нет свежей копии" {
					t.Fatal("fresh copy treated as stale")
				}
			}
		}
	}
	if !found {
		t.Fatal("stale check hidden by recent copy")
	}
}
