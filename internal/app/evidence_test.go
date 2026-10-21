package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type failingEvidenceStore struct {
	app.Store
	fail bool
}
type failingEvidenceTx struct {
	app.Tx
	fail bool
}

var errEvidence = errors.New("evidence storage unavailable")

func (t failingEvidenceTx) List(kind string) ([]json.RawMessage, error) {
	if t.fail && kind == "document" {
		return nil, errEvidence
	}
	return t.Tx.List(kind)
}
func (s *failingEvidenceStore) Write(ctx context.Context, fn func(app.Tx) error) error {
	return s.Store.Write(ctx, func(tx app.Tx) error { return fn(failingEvidenceTx{tx, s.fail}) })
}
func (s *failingEvidenceStore) Read(ctx context.Context, fn func(app.Tx) error) error {
	return s.Store.Read(ctx, func(tx app.Tx) error { return fn(failingEvidenceTx{tx, s.fail}) })
}
func TestEvidenceReadFailurePreventsDraftAndApproval(t *testing.T) {
	ctx := context.Background()
	store := &failingEvidenceStore{Store: testkit.NewStore()}
	svc := app.New(store, time.Now, testkit.IDs())
	editor := domain.Principal{Role: "editor", ID: "editor"}
	admin := domain.Principal{Role: "admin", ID: "admin"}
	plan := domain.Document{Kind: "maintenance_plan", Title: "Plan", Body: "Review"}
	draft, err := svc.EditDocument(ctx, editor, plan, 0)
	if err != nil {
		t.Fatal(err)
	}
	store.fail = true
	if _, err := svc.EditDocument(ctx, editor, plan, 0); !errors.Is(err, errEvidence) {
		t.Fatalf("draft accepted incomplete evidence: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, admin, draft.ID, draft.Revision); !errors.Is(err, errEvidence) {
		t.Fatalf("approval ignored unavailable evidence: %v", err)
	}
	if _, err := svc.Document(ctx, admin, draft.ID, 0); !errors.Is(err, errEvidence) {
		t.Fatalf("read ignored unavailable evidence: %v", err)
	}
}

func TestPlanRevalidationTracksVersionsNotRefreshHeartbeat(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin", ID: "human"}
	editor := domain.Principal{Role: "editor", ID: "editor"}
	n, err := svc.SaveNode(ctx, admin, domain.Node{Name: "host"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.Principal{Role: "agent", Instance: "agent", Nodes: []string{n.ID}}
	report := func(sequence uint64, version string) {
		data, _ := json.Marshal(domain.PackageData{Packages: []domain.Package{{Name: "foo", Version: version, Source: "foo", SourceVersion: version}}, MetadataRefresh: &domain.APTRefresh{LastAttempt: now, LastSuccess: now, Status: "success"}})
		_, err := svc.IngestReport(ctx, principal, domain.Report{SchemaVersion: 1, ID: now.String(), NodeID: n.ID, Instance: "agent", Sequence: sequence, ObservedAt: now, Collectors: map[string]domain.Section{"packages": {Status: "ok", Complete: true, ObservedAt: now, Data: data}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	report(1, "1")
	plan, err := svc.EditDocument(ctx, editor, domain.Document{Title: "Plan", Kind: "maintenance_plan", Body: "Check version", Nodes: []string{n.ID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	report(2, "1")
	read, err := svc.Document(ctx, admin, plan.ID, 0)
	if err != nil || read.NeedsRevalidation {
		t.Fatal("heartbeat invalidated unchanged packages", read.NeedsRevalidation, err)
	}
	now = now.Add(time.Hour)
	report(3, "2")
	read, err = svc.Document(ctx, admin, plan.ID, 0)
	if err != nil || !read.NeedsRevalidation {
		t.Fatal("changed package did not invalidate plan", err)
	}
}
