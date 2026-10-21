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
