package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestTopologySuggestionsRequireConfirmedKeysAndPreserveRouteIdentity(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	key := func(value byte) string {
		b := make([]byte, 32)
		for i := range b {
			b[i] = value
		}
		return base64.StdEncoding.EncodeToString(b)
	}
	a, err := svc.SaveNode(ctx, admin, domain.Node{Name: "source", WireGuardPublicKeys: []string{key(1)}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.SaveNode(ctx, admin, domain.Node{Name: "target", Addresses: []string{"192.0.2.1"}, WireGuardPublicKeys: []string{key(2)}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	wireguard, _ := json.Marshal(map[string]string{"public-key": "wg0\t" + key(1), "peers": "wg0\t" + key(2) + "\nwg0\t" + key(3), "endpoints": "wg0\t" + key(3) + "\t192.0.2.1:51820"})
	report := domain.Report{SchemaVersion: 1, ID: "r", NodeID: a.ID, Instance: "agent", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"wireguard": {Status: "ok", Complete: true, ObservedAt: now, Data: wireguard}, "network": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"routes":[{"dst":"default","gateway":"192.0.2.1","dev":"eth0","table":"main"},{"dst":"default","gateway":"192.0.2.1","dev":"wg0","table":100}]}`)}}}
	if _, err = svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{a.ID}, Instance: "agent"}, report); err != nil {
		t.Fatal(err)
	}
	suggestions, err := svc.RelationSuggestions(ctx, admin, a.ID)
	if err != nil || len(suggestions) != 3 {
		t.Fatal(suggestions, err)
	}
	relations, err := svc.Relations(ctx, admin)
	if err != nil || len(relations) != 0 {
		t.Fatal("autodetect modified manual topology", relations, err)
	}
	var selected domain.Relation
	graph, err := svc.Topology(ctx, admin)
	if err != nil || len(graph.UnmanagedPeers) != 1 || graph.UnmanagedPeers[0].PublicKey != key(3) || len(graph.Relations) != 0 {
		t.Fatal("unknown peer must stay outside inventory and manual graph", graph, err)
	}
	for _, s := range suggestions {
		if s.Target != b.ID || s.ObservedAt != now || s.Provenance != "observed" {
			t.Fatal(s)
		}
		if s.Type == "tunnel_peer" {
			selected = domain.Relation{Source: s.Source, Target: s.Target, Type: s.Type, Name: s.Name}
		}
	}
	if _, err = svc.SaveRelation(ctx, admin, selected); err != nil {
		t.Fatal(err)
	}
	suggestions, err = svc.RelationSuggestions(ctx, admin, a.ID)
	if err != nil || len(suggestions) != 2 {
		t.Fatal("accepted relation still suggested", suggestions, err)
	}
	b.WireGuardPublicKeys = nil
	if _, err = svc.SaveNode(ctx, admin, b, b.Revision); err != nil {
		t.Fatal(err)
	}
	// A second node with the same address must make gateway inference ambiguous.
	if _, err = svc.SaveNode(ctx, admin, domain.Node{Name: "ambiguous", Addresses: []string{"192.0.2.1"}}, 0); err != nil {
		t.Fatal(err)
	}
	suggestions, err = svc.RelationSuggestions(ctx, admin, a.ID)
	if err != nil || len(suggestions) != 0 {
		t.Fatal("ambiguous binding inferred", suggestions, err)
	}
}

func TestSuggestionsExpireWithoutDeletingManualRelations(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	a, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "source"}, 0)
	b, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "gateway", Addresses: []string{"2001:db8::1/64"}}, 0)
	report := domain.Report{SchemaVersion: 1, ID: "r", NodeID: a.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"network": {Status: "ok", Complete: true, ObservedAt: now, Data: json.RawMessage(`{"ipv6/route":[{"dst-address":"::/0","gateway":"2001:db8::1%ether1","routing-table":"main","active":"true"}]}`)}}}
	if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{a.ID}, Instance: "i"}, report); err != nil {
		t.Fatal(err)
	}
	suggestions, err := svc.RelationSuggestions(ctx, domain.Principal{Role: "reader"}, b.ID)
	if err != nil || len(suggestions) != 1 {
		t.Fatal(suggestions, err)
	}
	if _, err = svc.RelationSuggestions(ctx, domain.Principal{Role: "agent"}, a.ID); err != app.ErrForbidden {
		t.Fatal(err)
	}
	if _, err = svc.SaveRelation(ctx, admin, domain.Relation{Source: a.ID, Target: b.ID, Type: "depends_on"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(25 * time.Hour)
	suggestions, err = svc.RelationSuggestions(ctx, admin, "")
	if err != nil || len(suggestions) != 0 {
		t.Fatal(suggestions, err)
	}
	relations, err := svc.Relations(ctx, admin)
	if err != nil || len(relations) != 1 {
		t.Fatal(relations, err)
	}
}

func TestConfirmedWireGuardKeyValidation(t *testing.T) {
	svc := app.New(testkit.NewStore(), time.Now, testkit.IDs())
	ctx := context.Background()
	admin := domain.Principal{Role: "admin"}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, keys := range [][]string{{"not-a-key"}, {key, key}, {key + "\n"}} {
		if _, err := svc.SaveNode(ctx, admin, domain.Node{Name: "bad", WireGuardPublicKeys: keys}, 0); err != app.ErrInvalid {
			t.Fatalf("accepted invalid binding: %v", err)
		}
	}
}

func TestRouterOSWireGuardSuggestionsDeduplicateReverseObservations(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	key := func(v byte) string {
		raw := make([]byte, 32)
		raw[0] = v
		return base64.StdEncoding.EncodeToString(raw)
	}
	a, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "a", WireGuardPublicKeys: []string{key(1)}}, 0)
	b, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "b", WireGuardPublicKeys: []string{key(2)}}, 0)
	for _, pair := range []struct {
		node          domain.Node
		local, remote string
	}{{a, key(1), key(2)}, {b, key(2), key(1)}} {
		raw, _ := json.Marshal(map[string]any{"interface/wireguard": []map[string]string{{"name": "wg0", "public-key": pair.local}}, "interface/wireguard/peers": []map[string]string{{"interface": "wg0", "public-key": pair.remote}}})
		r := domain.Report{SchemaVersion: 1, ID: pair.node.ID, NodeID: pair.node.ID, Instance: "i", Sequence: 1, ObservedAt: now, Collectors: map[string]domain.Section{"wireguard": {Status: "ok", Complete: true, ObservedAt: now, Data: raw}}}
		if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{pair.node.ID}, Instance: "i"}, r); err != nil {
			t.Fatal(err)
		}
	}
	suggestions, err := svc.RelationSuggestions(ctx, admin, "")
	if err != nil || len(suggestions) != 1 {
		t.Fatal(suggestions, err)
	}
	if _, err = svc.SaveNode(ctx, admin, domain.Node{Name: "ambiguous-key", WireGuardPublicKeys: []string{key(2)}}, 0); err != nil {
		t.Fatal(err)
	}
	suggestions, err = svc.RelationSuggestions(ctx, admin, "")
	if err != nil || len(suggestions) != 0 {
		t.Fatal("ambiguous key inferred", suggestions, err)
	}
}
