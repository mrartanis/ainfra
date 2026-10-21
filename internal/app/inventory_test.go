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

func TestDuplicateRelationPreservesCreationTimeAndAudit(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	a, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "a"}, 0)
	b, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "b"}, 0)
	r, err := svc.SaveRelation(ctx, admin, domain.Relation{Source: a.ID, Target: b.ID, Type: "tunnel_peer", Name: "wg"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.CreatedAt.Equal(now) || !r.UpdatedAt.Equal(now) {
		t.Fatal("missing creation time", r)
	}
	before, _ := svc.Events(ctx, admin)
	now = now.Add(time.Hour)
	repeated, err := svc.SaveRelation(ctx, admin, domain.Relation{Source: b.ID, Target: a.ID, Type: "tunnel_peer", Name: "wg"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := svc.Events(ctx, admin)
	if repeated != r || len(after) != len(before) {
		t.Fatal("retry changed relation or audit", repeated, r, len(before), len(after))
	}
}

func TestInvalidPaymentScheduleIsRejectedBeforeSaving(t *testing.T) {
	svc := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	for _, payment := range []domain.Payment{{NextDue: time.Now(), Interval: 0, Unit: "months", Timezone: "UTC"}, {NextDue: time.Now(), Interval: 1, Unit: "minutes", Timezone: "UTC"}, {NextDue: time.Now(), Interval: 1, Unit: "months", Timezone: "Not/AZone"}, {Interval: 1, Unit: "months", Timezone: "UTC"}} {
		if _, err := svc.SaveNode(context.Background(), admin, domain.Node{Name: "bad schedule", Payment: &payment}, 0); err != app.ErrInvalid {
			t.Fatal("invalid schedule saved", payment, err)
		}
	}
}
