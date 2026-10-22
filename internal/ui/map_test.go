package ui

import (
	"ainfra/internal/domain"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMapNestedHostingPeersAndSafeRendering(t *testing.T) {
	topology := domain.Topology{Nodes: []domain.Node{
		{ID: "host", Name: "Host", Kind: "physical"},
		{ID: "vm", Name: "VM", Kind: "vm"},
		{ID: "container", Name: "<script>bad()</script>", Services: []domain.Service{{Name: "Jellyfin", Exposure: "public"}}},
	}, Relations: []domain.Relation{
		{Source: "vm", Target: "host", Type: "runs_on"},
		{Source: "container", Target: "vm", Type: "runs_on"},
	}, UnmanagedPeers: []domain.WireGuardPeer{{NodeID: "vm", Interface: "wg0", PublicKey: "unknown-key"}}}
	view := buildMap(topology, "all")
	x := map[string]int{}
	for _, n := range view.Nodes {
		x[n.ID] = n.X
	}
	if x["host"] >= x["vm"] || x["vm"] >= x["container"] || len(view.Nodes) != 4 || len(view.Edges) != 3 {
		t.Fatalf("%+v", view)
	}
	wg := buildMap(topology, "tunnel_peer")
	if len(wg.Edges) != 1 || !wg.Edges[0].Observed || wg.Edges[0].Type != "tunnel_peer" {
		t.Fatalf("%+v", wg)
	}
	u := New("http://unused", nil, false, time.Now)
	response := httptest.NewRecorder()
	u.render(response, map[string]any{"Page": "map", "Map": view})
	body := response.Body.String()
	for _, want := range []string{"</html>", "unknown-key", "Jellyfin", "&lt;script&gt;", "Запущен на", "WireGuard"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, "ZgotmplZ") {
		t.Fatal(body)
	}
}

func TestMapCycleDoesNotHangAndMultipleParentsUseDeepestLevel(t *testing.T) {
	topology := domain.Topology{Nodes: []domain.Node{{ID: "a"}, {ID: "b"}, {ID: "c"}}, Relations: []domain.Relation{{Source: "b", Target: "a", Type: "runs_on"}, {Source: "c", Target: "b", Type: "runs_on"}, {Source: "c", Target: "a", Type: "runs_on"}}}
	view := buildMap(topology, "all")
	x := map[string]int{}
	for _, n := range view.Nodes {
		x[n.ID] = n.X
	}
	if x["c"] <= x["b"] || x["b"] <= x["a"] {
		t.Fatal(x)
	}
	topology.Relations = append(topology.Relations, domain.Relation{Source: "a", Target: "c", Type: "runs_on"})
	if len(buildMap(topology, "all").Nodes) != 3 {
		t.Fatal("lost cycle nodes")
	}
}

func TestPackageStateDoesNotFlagEqualOrOlderCandidates(t *testing.T) {
	for _, tc := range []struct{ installed, candidate, want string }{
		{"0.1.6-2build1", "0.1.6-2build1", "Совпадает с кандидатом"},
		{"22.07.5-2ubuntu1.6", "22.07.5-2ubuntu1.6", "Совпадает с кандидатом"},
		{"1", "0:1-0", "Совпадает с кандидатом"},
		{"1", "2", "Доступно обновление"},
		{"2", "1", "Кандидат старее установленного"},
		{"1", "", "Кандидат неизвестен"},
	} {
		if got := packageState(map[string]any{"version": tc.installed, "candidate": tc.candidate}); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
}
