package linuxcollect_test

import (
	"ainfra/internal/adapters/linuxcollect"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type refreshRunner struct {
	err   error
	calls []string
}

func (r *refreshRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append([]string{name}, args...)
	return nil, r.err
}

func TestManagedRefreshPreservesLastSuccessAfterFailure(t *testing.T) {
	now := time.Now().UTC()
	var state []byte
	read := func(string) ([]byte, error) {
		if state == nil {
			return nil, errors.New("missing")
		}
		return state, nil
	}
	write := func(_ string, b []byte) error { state = append([]byte(nil), b...); return nil }
	runner := &refreshRunner{}
	if err := linuxcollect.RefreshAPT(context.Background(), runner, read, write, func() time.Time { return now }, "status.json"); err != nil {
		t.Fatal(err)
	}
	first := now
	now = now.Add(time.Hour)
	runner.err = errors.New("apt stderr contains secrets")
	if err := linuxcollect.RefreshAPT(context.Background(), runner, read, write, func() time.Time { return now }, "status.json"); err == nil {
		t.Fatal("failure hidden")
	}
	var v map[string]any
	if err := json.Unmarshal(state, &v); err != nil {
		t.Fatal(err)
	}
	if v["last_success"] != first.Format(time.RFC3339Nano) || v["status"] != "error" || v["last_attempt"] != now.Format(time.RFC3339Nano) {
		t.Fatal(v)
	}
	if string(state) == "" {
		t.Fatal("status missing")
	}
	if runner.calls[0] != "/usr/bin/apt-get" || runner.calls[len(runner.calls)-1] != "update" {
		t.Fatal(runner.calls)
	}
	for _, arg := range runner.calls {
		if arg == "upgrade" || arg == "install" {
			t.Fatal(runner.calls)
		}
	}
}

func TestCollectorReadsManagedRefreshWithoutRunningIt(t *testing.T) {
	now := time.Now().UTC()
	state, _ := json.Marshal(map[string]any{"last_attempt": now, "last_success": now.Add(-time.Hour), "status": "error"})
	runner := &refreshRunner{}
	read := func(path string) ([]byte, error) {
		if path == linuxcollect.APTRefreshPath {
			return state, nil
		}
		return []byte("ID=debian\n"), nil
	}
	c := linuxcollect.New(runner, read, func() time.Time { return now })
	section := c.Packages(context.Background())
	var data struct {
		Refresh map[string]any `json:"metadata_refresh"`
	}
	if err := json.Unmarshal(section.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Refresh["status"] != "error" {
		t.Fatal(string(section.Data))
	}
	for _, arg := range runner.calls {
		if arg == "update" || arg == "/usr/bin/apt-get" {
			t.Fatal("collector refreshed APT")
		}
	}
}
