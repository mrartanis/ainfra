package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"errors"
	"testing"
	"time"
)

func TestDocumentRevisionsAndHumanApproval(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	ed := domain.Principal{ID: "llm", Role: "editor"}
	ad := domain.Principal{ID: "human", Role: "admin"}
	d, e := s.EditDocument(ctx, ed, domain.Document{Title: "Plan", Kind: "maintenance_plan", Body: "backup first"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ApprovePlan(ctx, ed, d.ID, d.Revision); !errors.Is(e, app.ErrForbidden) {
		t.Fatal(e)
	}
	d, e = s.ApprovePlan(ctx, ad, d.ID, d.Revision)
	if e != nil || d.Status != "approved" {
		t.Fatalf("%+v %v", d, e)
	}
	d.Body = "changed"
	d, e = s.EditDocument(ctx, ed, d, d.Revision)
	if e != nil || d.Status != "draft" || d.ApprovedRevision != 0 {
		t.Fatalf("%+v %v", d, e)
	}
	if _, e = s.EditDocument(ctx, ed, d, 1); !errors.Is(e, app.ErrConflict) {
		t.Fatal(e)
	}
	old, e := s.Document(ctx, ed, d.ID, 1)
	if e != nil || old.Body != "backup first" {
		t.Fatal(old, e)
	}
}
func TestTokenScopesAndRevocation(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	a := domain.Principal{Role: "admin", ID: "human"}
	tok, meta, e := s.IssueToken(ctx, a, domain.Principal{Role: "reader"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Authenticate(ctx, tok)
	if e != nil || p.Role != "reader" {
		t.Fatal(p, e)
	}
	if e = s.RevokeToken(ctx, a, meta.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, tok); !errors.Is(e, app.ErrForbidden) {
		t.Fatal(e)
	}
}

func TestReenrollRevokesOldAgent(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	n, _ := s.SaveNode(ctx, admin, domain.Node{Name: "n"}, 0)
	old, _, e := s.IssueToken(ctx, admin, domain.Principal{Role: "agent", Nodes: []string{n.ID}, Instance: "old"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Reenroll(ctx, admin, n.ID, "new", n.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, old); !errors.Is(e, app.ErrForbidden) {
		t.Fatal("old token still active")
	}
	n, _ = s.Node(ctx, admin, n.ID)
	if n.Instance != "new" || n.Sequence != 0 {
		t.Fatal(n)
	}
}

func TestPlanTerminalStatusRequiresAdminAndApprovedRevision(t *testing.T) {
	svc := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	editor := domain.Principal{Role: "editor", ID: "editor"}
	admin := domain.Principal{Role: "admin", ID: "human"}
	draft, err := svc.EditDocument(ctx, editor, domain.Document{Title: "Plan", Kind: "maintenance_plan", Body: "Manual maintenance"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SetPlanStatus(ctx, admin, draft.ID, 1, "completed"); !errors.Is(err, app.ErrConflict) {
		t.Fatal("draft completed", err)
	}
	if _, err = svc.ApprovePlan(ctx, admin, draft.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SetPlanStatus(ctx, editor, draft.ID, 1, "completed"); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("editor completed", err)
	}
	if _, err = svc.SetPlanStatus(ctx, admin, draft.ID, 2, "completed"); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale revision completed", err)
	}
	done, err := svc.SetPlanStatus(ctx, admin, draft.ID, 1, "completed")
	if err != nil || done.Status != "completed" {
		t.Fatal(done, err)
	}
	if _, err = svc.ApprovePlan(ctx, admin, draft.ID, 1); !errors.Is(err, app.ErrConflict) {
		t.Fatal("completed plan reopened by approval", err)
	}
	draft.ID = ""
	draft, err = svc.EditDocument(ctx, editor, draft, 0)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.SetPlanStatus(ctx, admin, draft.ID, 1, "cancelled")
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatal(cancelled, err)
	}
}
