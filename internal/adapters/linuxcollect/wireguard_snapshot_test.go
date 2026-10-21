package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestWireGuardSnapshotPreservesObservationTimeWithoutCommands(t *testing.T) {
	now := time.Now().UTC()
	observed := now.Add(-time.Hour)
	runner := &fakeRunner{}
	raw, _ := json.Marshal(map[string]any{"status": "ok", "complete": true, "observed_at": observed, "data": map[string]string{"public-key": "wg0 public", "peers": "peer", "endpoints": "endpoint", "allowed-ips": "10.0.0.0/24", "latest-handshakes": "0"}})
	c := linuxcollect.New(runner, func(string) ([]byte, error) { return raw, nil }, func() time.Time { return now }, "/snapshot.json")
	section := c.WireGuard(context.Background())
	if section.Status != "ok" || !section.ObservedAt.Equal(observed) || len(runner.calls) != 0 {
		t.Fatal(section, runner.calls)
	}
	raw = []byte(`{"status":"ok","complete":true,"observed_at":"2026-10-21T12:00:00Z","data":{"private-key":"secret"}}`)
	if section = c.WireGuard(context.Background()); section.Status != "error" || len(section.Data) > 0 {
		t.Fatal("unsafe helper snapshot accepted", section)
	}
}
