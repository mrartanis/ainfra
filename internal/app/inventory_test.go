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

func TestRelationsAndCalendarPayment(t *testing.T) {
	now := time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC)
	s := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	ctx := context.Background()
	p := domain.Principal{Role: "admin"}
	a, _ := s.SaveNode(ctx, p, domain.Node{Name: "A", Payment: &domain.Payment{NextDue: now, Interval: 1, Unit: "months", Timezone: "UTC"}}, 0)
	b, _ := s.SaveNode(ctx, p, domain.Node{Name: "B"}, 0)
	if _, e := s.SaveRelation(ctx, p, domain.Relation{Source: a.ID, Target: b.ID, Type: "runs_on"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveRelation(ctx, p, domain.Relation{Source: b.ID, Target: a.ID, Type: "runs_on"}); !errors.Is(e, app.ErrConflict) {
		t.Fatalf("cycle: %v", e)
	}
	n, e := s.Pay(ctx, p, a.ID, a.Revision)
	if e != nil || n.Payment.NextDue.Day() != 29 || n.Payment.NextDue.Month() != 2 {
		t.Fatal(n.Payment, e)
	}
}
