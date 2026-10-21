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
	// The form was opened before the report. Saving its manual description
	// must preserve the newer check result, despite the unchanged manual revision.
	n.Backups[0].Label = "edited description"
	n, e = s.SaveNode(ctx, admin, n, n.Revision)
	if e != nil {
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

func TestObservedPaymentDoesNotPayOrOverwriteManualSchedule(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	manualDue := now.AddDate(0, 1, 0)
	observedDue := now.AddDate(0, 0, 3)
	n, err := svc.SaveNode(ctx, admin, domain.Node{Name: "billing", Payment: &domain.Payment{NextDue: manualDue, Interval: 1, Unit: "months", Timezone: "UTC"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	send := func(seq uint64, status string, at time.Time) {
		data, _ := json.Marshal([]app.Check{{SchemaVersion: 1, ID: "provider-invoice", Kind: "payment", Status: status, ObservedAt: at, Data: mustJSON(t, app.PaymentResult{NextDue: observedDue, Amount: "10", Currency: "EUR", Evidence: "invoice fixture"})}})
		r := domain.Report{SchemaVersion: 1, ID: fmt.Sprint(seq), NodeID: n.ID, Instance: "i", Sequence: seq, ObservedAt: now, Collectors: map[string]domain.Section{"checks": {Status: "partial", Complete: false, ObservedAt: now, Data: data}}}
		if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}, r); err != nil {
			t.Fatal(err)
		}
	}
	send(1, "ok", now)
	got, err := svc.Node(ctx, admin, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	payment := got.PaymentChecks["provider-invoice"]
	if !payment.NextDue.Equal(observedDue) || !got.Payment.NextDue.Equal(manualDue) || !got.Payment.LastPaid.IsZero() {
		t.Fatal(got.Payment, payment)
	}
	successAt := now
	now = now.Add(time.Hour)
	send(2, "error", now)
	attention, err := svc.Attention(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	warned := false
	for _, reason := range attention[0].Reasons {
		if strings.Contains(reason, "provider-invoice") && strings.Contains(reason, "неудачна") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("payment check failure hidden from attention", attention)
	}
	// A stale manual form cannot erase observed payment facts.
	n.Purpose = "edited"
	got, err = svc.SaveNode(ctx, admin, n, n.Revision)
	if err != nil {
		t.Fatal(err)
	}
	payment = got.PaymentChecks["provider-invoice"]
	if payment.Status != "error" || !payment.LastSuccessAt.Equal(successAt) || !payment.NextDue.Equal(observedDue) {
		t.Fatal(payment)
	}
	send(3, "ok", successAt.Add(-time.Minute))
	got, _ = svc.Node(ctx, admin, n.ID)
	if got.PaymentChecks["provider-invoice"].Status != "error" {
		t.Fatal("old success hid latest failure")
	}
	cleared := domain.Report{SchemaVersion: 1, ID: "clear", NodeID: n.ID, Instance: "i", Sequence: 4, ObservedAt: now, Collectors: map[string]domain.Section{"checks": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`[]`)}}}
	if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}, cleared); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Node(ctx, admin, n.ID)
	if len(got.PaymentChecks) != 0 || !got.Payment.NextDue.Equal(manualDue) {
		t.Fatal("removed check retained or manual schedule changed", got)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCheckCannotPoisonFreshnessWithFutureTimestamp(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	n, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "checks"}, 0)
	data := mustJSON(t, []app.Check{{SchemaVersion: 1, ID: "invoice", Kind: "payment", Status: "ok", ObservedAt: now.Add(time.Hour), Data: mustJSON(t, app.PaymentResult{NextDue: now.AddDate(0, 1, 0)})}})
	report := domain.Report{SchemaVersion: 1, ID: "future-check", NodeID: n.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"checks": {Status: "ok", Complete: true, ObservedAt: now, Data: data}}}
	if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "i"}, report); err != app.ErrInvalid {
		t.Fatal("future check accepted", err)
	}
}

func TestFailedPaymentCheckDoesNotRequireInventedDueDate(t *testing.T) {
	raw := json.RawMessage(`[{"schema_version":1,"check_id":"invoice","kind":"payment","observed_at":"2026-10-21T12:00:00Z","status":"error","data":{"evidence":"provider unavailable"}}]`)
	if err := app.ValidateChecks(raw); err != nil {
		t.Fatal(err)
	}
}
