package feeds_test

import (
	"ainfra/internal/adapters/feeds"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceRecoversOrphansWithoutTouchingActiveWorkOrOtherFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "staging")
	workspace, err := feeds.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspace.NewStage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = feeds.OpenWorkspace(root); err == nil {
		t.Fatal("concurrent workspace acquired")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			found = true
		}
	}
	if !found {
		t.Fatal("active stage was removed")
	}
	// Releasing the process lock without stage.Close models a crashed importer.
	if err = workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "operator-note"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err = os.WriteFile(filepath.Join(external, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(external, filepath.Join(root, "infra-feed-symlink")); err != nil {
		t.Fatal(err)
	}
	recovered, err := feeds.OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	entries, err = os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("orphan remains: %s", entry.Name())
		}
	}
	for _, path := range []string{filepath.Join(root, "operator-note"), filepath.Join(external, "keep")} {
		if _, err = os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	_ = stage.Close()
}
