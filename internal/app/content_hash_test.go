package app_test

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"ainfra/internal/testkit"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSectionContentHashIgnoresEnvelopeAndPreservesIntegerPrecision(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()
	svc := app.New(testkit.NewStore(), func() time.Time { return now }, testkit.IDs())
	admin := domain.Principal{Role: "admin"}
	node, _ := svc.SaveNode(ctx, admin, domain.Node{Name: "hash fixture"}, 0)
	send := func(id string, seq uint64, raw string) domain.Node {
		r := domain.Report{SchemaVersion: 1, ID: id, NodeID: node.ID, Instance: "i", Sequence: seq, ObservedAt: now, Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: now, ContentHash: "untrusted", Data: json.RawMessage(raw)}}}
		if _, err := svc.IngestReport(ctx, domain.Principal{Role: "agent", Nodes: []string{node.ID}, Instance: "i"}, r); err != nil {
			t.Fatal(err)
		}
		n, err := svc.Node(ctx, admin, node.ID)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	first := send("a", 1, `{"id":"debian","memory_bytes":9007199254740993}`)
	hash := first.Observations["os"].ContentHash
	if len(hash) != 64 || hash == "untrusted" {
		t.Fatal("hash not computed by server", hash)
	}
	now = now.Add(time.Hour)
	same := send("b", 2, `{ "memory_bytes":9007199254740993, "id":"debian" }`)
	if same.Observations["os"].ContentHash != hash || !same.Observations["os"].ObservedAt.Equal(now) {
		t.Fatal("envelope/formatting changed content hash", same)
	}
	changed := send("c", 3, `{"id":"debian","memory_bytes":9007199254740992}`)
	if changed.Observations["os"].ContentHash == hash {
		t.Fatal("integer precision lost in hash")
	}
}
