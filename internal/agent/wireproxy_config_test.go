package agent_test

import (
	"ainfra/internal/agent"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWireProxyConfigRequiresPublicBindingAndLoopbackSource(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, tc := range []struct {
		url, key, snapshot string
		valid              bool
	}{
		{"http://127.0.0.1:9080/metrics", key, "", true},
		{"http://127.0.0.1:9080/metrics", "", "", false},
		{"", key, "", false},
		{"http://example.org:9080/metrics", key, "", false},
		{"http://127.0.0.1:9080/metrics", key, "/snapshot.json", false},
	} {
		c := agent.Config{NodeID: "node", Instance: "instance", Server: "https://infra.example.org", TokenFile: "/token", StateDir: "/state", WireProxyMetricsURL: tc.url, WireProxyPublicKey: tc.key, WireGuardSnapshotFile: tc.snapshot}
		raw, _ := json.Marshal(c)
		path := filepath.Join(t.TempDir(), "agent.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := agent.LoadConfig(path); (err == nil) != tc.valid {
			t.Fatal(tc, err)
		}
	}
}
