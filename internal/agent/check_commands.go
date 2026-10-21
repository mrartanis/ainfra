package agent

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type CheckCommand struct {
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	OutputLimit    int      `json:"output_limit"`
}
type CheckRunner interface {
	Run(context.Context, []string, int) ([]byte, error)
}
type ExecCheckRunner struct{}
type checkOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *checkOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, errors.New("check_output_limit")
	}
	return b.buffer.Write(p)
}
func validCheckCommand(c CheckCommand) bool {
	if len(c.Argv) == 0 || len(c.Argv) > 128 || !filepath.IsAbs(c.Argv[0]) || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 60 || c.OutputLimit < 1 || c.OutputLimit > 1<<20 {
		return false
	}
	size := 0
	for _, arg := range c.Argv {
		size += len(arg)
	}
	return size <= 16384
}

// Local operator configuration is the sole executable allowlist. No shell
// interpolation, API-supplied commands, stderr or raw failure output is used.
func (ExecCheckRunner) Run(ctx context.Context, argv []string, limit int) ([]byte, error) {
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) || limit < 1 || limit > 1<<20 {
		return nil, errors.New("invalid_check_command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 250 * time.Millisecond
	out := &checkOutput{limit: limit}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return nil, errors.New("check_command_failed")
	}
	return out.buffer.Bytes(), nil
}
