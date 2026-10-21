package agent_test

import (
	"ainfra/internal/agent"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type checkRunner struct{ calls int }

func (r *checkRunner) Run(ctx context.Context, argv []string, limit int) ([]byte, error) {
	r.calls++
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	if argv[0] == "/local/fails" {
		return nil, errors.New("failure with private output")
	}
	return []byte(`{"schema_version":1,"check_id":"copy","kind":"backup","observed_at":"2026-10-21T12:00:00Z","status":"ok","data":{"last_attempt_at":"2026-10-21T12:00:00Z","last_attempt_status":"success"}}`), nil
}
func TestLocalCheckCommandsMergeSuccessAndTimeout(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	runner := &checkRunner{}
	config := agent.Config{NodeID: "node", Instance: "instance", StateDir: t.TempDir(), CheckCommands: []agent.CheckCommand{{Argv: []string{"/local/backup"}, TimeoutSeconds: 1, OutputLimit: 4096}, {Argv: []string{"/local/fails"}, TimeoutSeconds: 1, OutputLimit: 4096}}}
	a := agent.New(config, collector{}, &transport{}, func() time.Time { return now }, func() string { return "report" }, runner)
	r, err := a.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s := r.Collectors["checks"]
	var rows []json.RawMessage
	if json.Unmarshal(s.Data, &rows) != nil || len(rows) != 1 || s.Status != "partial" || s.Complete || runner.calls != 2 {
		t.Fatal(s, runner.calls)
	}
}
func TestCheckExecutorBoundsOutputAndTime(t *testing.T) {
	runner := agent.ExecCheckRunner{}
	if _, err := runner.Run(context.Background(), []string{"/usr/bin/printf", "%4096s", "x"}, 32); err == nil {
		t.Fatal("unbounded output")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := runner.Run(ctx, []string{"/bin/sleep", "10"}, 32); err == nil || time.Since(start) > time.Second {
		t.Fatal("timeout failed", err)
	}
}
