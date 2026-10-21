package main

import (
	"ainfra/internal/agent"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHelpWorksWithoutConfiguration(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	for _, args := range [][]string{{"help"}, {"--help"}, {"collect", "--help"}} {
		os.Args = append([]string{"infra-agent"}, args...)
		if err := run(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}
func TestCheckAliasValidatesConfigurationWithoutReadingCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	raw, _ := json.Marshal(agent.Config{NodeID: "node", Instance: "instance", Server: "http://127.0.0.1:1", StateDir: dir, TokenFile: filepath.Join(dir, "not-created.token")})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"infra-agent", "check", "--config", path}
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
