package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"testing"
	"time"
)

func TestAttentionFiltersProviderAndAllHostedDescendants(t *testing.T) {
	s := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	p := domain.Principal{Role: "admin"}
	host, e := s.SaveNode(ctx, p, domain.Node{Name: "host", Labels: map[string]string{"provider": "ExampleProvider"}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	vm, _ := s.SaveNode(ctx, p, domain.Node{Name: "vm"}, 0)
	container, _ := s.SaveNode(ctx, p, domain.Node{Name: "container"}, 0)
	unknown, _ := s.SaveNode(ctx, p, domain.Node{Name: "unknown"}, 0)
	for _, r := range []domain.Relation{{Source: vm.ID, Target: host.ID, Type: "runs_on"}, {Source: container.ID, Target: vm.ID, Type: "runs_on"}} {
		if _, e = s.SaveRelation(ctx, p, r); e != nil {
			t.Fatal(e)
		}
	}
	for _, tc := range []struct {
		provider, host string
		count          int
		first          string
	}{
		{"ExampleProvider", "", 3, "container"}, {"", host.ID, 3, "container"}, {"ExampleProvider", vm.ID, 2, "container"},
		{"UNKNOWN", "", 1, "unknown"}, {"UNKNOWN", host.ID, 0, ""}, {"", "missing", 0, ""}, {"", unknown.ID, 1, "unknown"},
	} {
		rows, e := s.QueryAttention(ctx, p, "stale", "name", tc.provider, tc.host)
		if e != nil || len(rows) != tc.count || (len(rows) > 0 && rows[0].Name != tc.first) {
			t.Fatalf("%+v: %+v %v", tc, rows, e)
		}
	}
}
