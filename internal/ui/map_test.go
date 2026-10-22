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
	if x["host"] >= x["vm"] || x["vm"] >= x["container"] || len(view.Nodes) != 5 || len(view.Edges) != 4 || view.NodeCount != 3 || view.ProviderCount != 1 || x["provider:UNKNOWN"] >= x["host"] {
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
	for _, want := range []string{"</html>", "unknown-key", "Jellyfin", "&lt;script&gt;", "Запущен на", "WireGuard", `src="/map.js" defer`, `data-map-zoom="in"`, `data-map-zoom="out"`, `data-map-zoom="fit"`, `data-map-zoom="reset"`} {
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
	if len(buildMap(topology, "all").Nodes) != 4 {
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

func TestProviderGroupsAndFilterOptions(t *testing.T) {
	topology := domain.Topology{Nodes: []domain.Node{{ID: "host", Name: "Host", Labels: map[string]string{"provider": "ExampleProvider"}}, {ID: "vm", Name: "VM"}, {ID: "ct", Name: "Container"}, {ID: "other", Name: "Unknown host"}}, Relations: []domain.Relation{{Source: "vm", Target: "host", Type: "runs_on"}, {Source: "ct", Target: "vm", Type: "runs_on"}}}
	view := buildMap(topology, "runs_on")
	if view.ProviderCount != 2 || view.NodeCount != 4 || len(view.Edges) != 4 {
		t.Fatal(view)
	}
	options := hostingFilters(topology.Nodes, topology.Relations)
	if len(options.Providers) != 2 || options.Providers[0] != "ExampleProvider" || options.Providers[1] != "UNKNOWN" || len(options.Hosts) != 3 {
		t.Fatal(options)
	}
	u := New("http://unused", nil, false, time.Now)
	w := httptest.NewRecorder()
	u.render(w, map[string]any{"Page": "map", "Map": view})
	if strings.Contains(w.Body.String(), `href="/nodes/provider:`) || !strings.Contains(w.Body.String(), "UNKNOWN") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	u.render(w, map[string]any{"Page": "nodes", "HostingFilters": options, "Provider": "ExampleProvider", "Host": "vm"})
	for _, want := range []string{`name="provider"`, `name="host"`, `value="vm" selected`, `value="ExampleProvider" selected`, "По выбранным фильтрам узлов нет"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %s: %s", want, w.Body.String())
		}
	}
}

func TestProviderBandsAndHostSubtreesNeverInterleave(t *testing.T) {
	topology := domain.Topology{Nodes: []domain.Node{
		{ID: "ha", Name: "Host A", Labels: map[string]string{"provider": "Provider A"}},
		{ID: "hb", Name: "Host B", Labels: map[string]string{"provider": "Provider A"}},
		{ID: "hc", Name: "Host C", Labels: map[string]string{"provider": "Provider B"}},
		{ID: "va", Name: "Z VM A"}, {ID: "ca", Name: "A container A"}, {ID: "vb", Name: "A VM B"}, {ID: "cb", Name: "Z container B"}, {ID: "vc", Name: "VM C"},
	}, Relations: []domain.Relation{
		{Source: "va", Target: "ha", Type: "runs_on"}, {Source: "ca", Target: "va", Type: "runs_on"},
		{Source: "vb", Target: "hb", Type: "runs_on"}, {Source: "cb", Target: "vb", Type: "runs_on"}, {Source: "vc", Target: "hc", Type: "runs_on"},
		{Source: "ca", Target: "vc", Type: "tunnel_peer"},
	}}
	view := buildMap(topology, "all")
	if len(view.Groups) != 2 {
		t.Fatal(view.Groups)
	}
	for i, g := range view.Groups {
		if i > 0 && g.Y-(view.Groups[i-1].Y+view.Groups[i-1].Height) < 200 {
			t.Fatal("provider gap too small", view.Groups)
		}
		for _, n := range view.Nodes {
			if n.Group == g.ID && (n.Y < g.Y+64 || n.Y+130 > g.Y+g.Height-32) {
				t.Fatal("node escaped provider band", g, n)
			}
		}
	}
	positions := map[string]mapNode{}
	for _, n := range view.Nodes {
		positions[n.ID] = n
	}
	for _, a := range []string{"ha", "va", "ca"} {
		for _, b := range []string{"hb", "vb", "cb"} {
			if positions[a].Y+130 >= positions[b].Y {
				t.Fatal("sibling host subtrees interleave", a, b, positions)
			}
		}
	}
	for i, a := range view.Nodes {
		for _, b := range view.Nodes[i+1:] {
			if a.X == b.X && a.Y < b.Y+130 && b.Y < a.Y+130 {
				t.Fatal("cards overlap", a, b)
			}
		}
	}
	for _, e := range view.Edges {
		if e.Type == "tunnel_peer" && (!strings.Contains(e.Path, " H ") || !strings.Contains(e.Path, " V ")) {
			t.Fatal("cross-provider link not routed outside cards", e)
		}
	}
	filtered := buildMap(topology, "runs_on")
	for _, n := range filtered.Nodes {
		p := positions[n.ID]
		if p.X != n.X || p.Y != n.Y {
			t.Fatal("edge filtering moved nodes", n, p)
		}
	}
	// Input order and maps must not affect the placement or primary DAG parent.
	for i, j := 0, len(topology.Nodes)-1; i < j; i, j = i+1, j-1 {
		topology.Nodes[i], topology.Nodes[j] = topology.Nodes[j], topology.Nodes[i]
	}
	for i, j := 0, len(topology.Relations)-1; i < j; i, j = i+1, j-1 {
		topology.Relations[i], topology.Relations[j] = topology.Relations[j], topology.Relations[i]
	}
	for _, n := range buildMap(topology, "all").Nodes {
		p := positions[n.ID]
		if p.X != n.X || p.Y != n.Y {
			t.Fatal("unstable layout", n, p)
		}
	}
}

func TestUnknownWireGuardPeersDoNotInheritObservedHostProvider(t *testing.T) {
	view := buildMap(domain.Topology{Nodes: []domain.Node{{ID: "edge", Labels: map[string]string{"provider": "ExampleSite"}}}, UnmanagedPeers: []domain.WireGuardPeer{{NodeID: "edge", PublicKey: "external"}}}, "all")
	if view.ProviderCount != 1 || len(view.Groups) != 2 || !view.Groups[1].External {
		t.Fatal(view)
	}
	for _, n := range view.Nodes {
		if n.Unmanaged && n.Group != "external-wireguard-peers" {
			t.Fatal("unknown placement shown as local provider", n)
		}
	}
}
