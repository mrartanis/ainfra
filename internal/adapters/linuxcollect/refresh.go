package linuxcollect

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"time"
)

const APTRefreshPath = "/var/lib/infra-apt-refresh/status.json"

// RefreshAPT is invoked only by the operator's separate opt-in root unit.
// Ordinary collection never calls it. Output from apt is not persisted.
func RefreshAPT(ctx context.Context, run Runner, read func(string) ([]byte, error), write func(string, []byte) error, now func() time.Time, path string) error {
	state := domain.APTRefresh{}
	if b, err := read(path); err == nil {
		_ = json.Unmarshal(b, &state)
	}
	state.LastAttempt = now()
	state.Status = "running"
	save := func() error {
		b, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return write(path, b)
	}
	if err := save(); err != nil {
		return err
	}
	_, err := run.Run(ctx, "/usr/bin/apt-get", "-o", "APT::Update::Error-Mode=any", "-o", "DPkg::Lock::Timeout=60", "update")
	state.Status = "success"
	if err != nil {
		state.Status = "error"
	} else {
		state.LastSuccess = now()
	}
	if e := save(); e != nil {
		return e
	}
	if err != nil {
		return errors.New("apt_refresh_failed")
	}
	return nil
}

func (c *Collector) aptRefresh() *domain.APTRefresh {
	b, err := c.read(APTRefreshPath)
	if err != nil || len(b) > 4096 {
		return nil
	}
	var state domain.APTRefresh
	if json.Unmarshal(b, &state) != nil || state.LastAttempt.IsZero() || state.LastAttempt.After(c.now().Add(5*time.Minute)) || state.LastSuccess.After(c.now().Add(5*time.Minute)) {
		return nil
	}
	if state.Status != "running" && state.Status != "success" && state.Status != "error" {
		return nil
	}
	return &state
}
