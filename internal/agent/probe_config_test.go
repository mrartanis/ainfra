package agent_test

import (
	"ainfra/internal/agent"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeConfigurationBoundaries(t *testing.T) {
	dir := t.TempDir()
	base := agent.Config{NodeID: "probe", Instance: "instance", Server: "https://infra.example", StateDir: dir, TokenFile: filepath.Join(dir, "token")}
	valid := agent.ProbeTarget{NodeID: "router", URL: "https://router.example", CredentialFile: filepath.Join(dir, "credentials"), TokenFile: filepath.Join(dir, "token")}
	for _, kind := range []string{"", "routeros", "keenetic", "unknown"} {
		c := base
		target := valid
		target.Type = kind
		c.ProbeTargets = []agent.ProbeTarget{target}
		raw, _ := json.Marshal(c)
		path := filepath.Join(dir, "config.json")
		os.WriteFile(path, raw, 0600)
		_, err := agent.LoadConfig(path)
		if (err != nil) != (kind == "unknown") {
			t.Fatal(kind, err)
		}
	}
	for _, node := range []string{"../escape", "..", "", "/absolute"} {
		c := base
		target := valid
		target.NodeID = node
		c.ProbeTargets = []agent.ProbeTarget{target}
		raw, _ := json.Marshal(c)
		path := filepath.Join(dir, "config.json")
		os.WriteFile(path, raw, 0600)
		if _, err := agent.LoadConfig(path); err == nil {
			t.Fatal("accepted unsafe state path", node)
		}
	}
}
