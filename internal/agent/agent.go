package agent

import (
	"ainfra/internal/apiclient"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	CheckCommands []CheckCommand `json:"check_commands"`
	NodeID        string         `json:"node_id"`
	Instance      string         `json:"agent_instance_id"`
	Server        string         `json:"server_url"`
	TokenFile     string         `json:"token_file"`
	CAFile        string         `json:"ca_file"`
	StateDir      string         `json:"state_dir"`
	Collectors    []string       `json:"collectors"`
	CheckFiles    []string       `json:"check_files"`
	ProbeTargets  []ProbeTarget  `json:"probe_targets"`
}
type ProbeTarget struct {
	NodeID         string `json:"node_id"`
	URL            string `json:"url"`
	CredentialFile string `json:"credential_file"`
	CAFile         string `json:"ca_file"`
	TokenFile      string `json:"token_file"`
}

func LoadConfig(path string) (c Config, err error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if c.NodeID == "" || c.Instance == "" || c.StateDir == "" || c.TokenFile == "" {
		return c, errors.New("missing config field")
	}
	if err = apiclient.ValidateURL(c.Server, true); err != nil {
		return
	}
	if !filepath.IsAbs(c.StateDir) || !filepath.IsAbs(c.TokenFile) {
		return c, errors.New("absolute paths required")
	}
	if len(c.Collectors) == 0 {
		c.Collectors = []string{"os", "packages", "network", "disks"}
	}
	for _, v := range c.Collectors {
		if v != "os" && v != "packages" && v != "network" && v != "wireguard" && v != "disks" {
			return c, errors.New("unknown collector")
		}
	}

	if len(c.CheckFiles)+len(c.CheckCommands) > 64 {
		return c, errors.New("too many checks")
	}
	for _, path := range c.CheckFiles {
		if !filepath.IsAbs(path) {
			return c, errors.New("absolute check path required")
		}
	}
	for _, command := range c.CheckCommands {
		if !validCheckCommand(command) {
			return c, errors.New("invalid check command")
		}
	}
	return
}

type Collector interface {
	Collect(context.Context, []string) map[string]domain.Section
}
type Transport interface {
	Send(context.Context, domain.Report) error
}
type APITransport struct{ Client *apiclient.Client }

func (t APITransport) Send(ctx context.Context, r domain.Report) error {
	var ack domain.Ack
	return t.Client.Do(ctx, "POST", "ingest/reports", r, &ack, 0)
}

type Agent struct {
	checkRunner CheckRunner
	config      Config
	collector   Collector
	transport   Transport
	now         func() time.Time
	id          func() string
	wait        func(context.Context, time.Duration) error
}

func New(c Config, col Collector, t Transport, now func() time.Time, id func() string, runners ...CheckRunner) *Agent {
	var runner CheckRunner
	if len(runners) > 0 {
		runner = runners[0]
	}
	return &Agent{config: c, collector: col, transport: t, now: now, id: id, wait: retryWait, checkRunner: runner}
}
func AtomicWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pending-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (a *Agent) lock() (func(), error) {
	if e := os.MkdirAll(a.config.StateDir, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(a.config.StateDir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("agent already running")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func (a *Agent) collect(ctx context.Context) (r domain.Report, err error) {
	seqPath := filepath.Join(a.config.StateDir, "sequence")
	b, e := os.ReadFile(seqPath)
	if e != nil && !os.IsNotExist(e) {
		return r, e
	}
	var seq uint64
	if len(b) > 0 {
		seq, e = strconv.ParseUint(string(b), 10, 64)
		if e != nil {
			return r, e
		}
	}
	seq++
	if e = AtomicWrite(seqPath, []byte(strconv.FormatUint(seq, 10))); e != nil {
		return r, e
	}
	r = domain.Report{SchemaVersion: 1, ID: a.id(), NodeID: a.config.NodeID, Instance: a.config.Instance, Sequence: seq, ObservedAt: a.now(), AgentVersion: "0.1.0", Collectors: a.collector.Collect(ctx, a.config.Collectors)}
	if len(a.config.CheckFiles)+len(a.config.CheckCommands) > 0 {
		r.Collectors["checks"] = a.checks(ctx)
	}
	return r, nil
}
func (a *Agent) Collect(ctx context.Context) (domain.Report, error) {
	unlock, e := a.lock()
	if e != nil {
		return domain.Report{}, e
	}
	defer unlock()
	return a.collect(ctx)
}
func retryWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// SetRetryWait injects the retry scheduler before the agent is run.
func (a *Agent) SetRetryWait(wait func(context.Context, time.Duration) error) { a.wait = wait }
func (a *Agent) Run(ctx context.Context) error {
	unlock, e := a.lock()
	if e != nil {
		return e
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	r, e := a.collect(ctx)
	if e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if len(b) > 16<<20 {
		return errors.New("report exceeds 16 MiB")
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if e = a.wait(ctx, time.Duration(attempt)*time.Second); e != nil {
				return e
			}
		}
		if e = a.transport.Send(ctx, r); e == nil {
			return nil
		}
	}
	return e
}
func (a *Agent) checks(ctx context.Context) domain.Section {
	rows := []json.RawMessage{}
	partial := false
	inputs := [][]byte{}
	for _, path := range a.config.CheckFiles {
		f, e := os.Open(path)
		if e != nil {
			partial = true
			continue
		}
		st, e := f.Stat()
		if e != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
			f.Close()
			partial = true
			continue
		}
		b := make([]byte, st.Size())
		_, e = io.ReadFull(f, b)
		f.Close()
		if e != nil {
			partial = true
			continue
		}
		inputs = append(inputs, b)
	}
	for _, command := range a.config.CheckCommands {
		if a.checkRunner == nil || !validCheckCommand(command) {
			partial = true
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
		b, err := a.checkRunner.Run(checkCtx, command.Argv, command.OutputLimit)
		cancel()
		if err != nil || len(b) > command.OutputLimit {
			partial = true
			continue
		}
		inputs = append(inputs, b)
	}
	seen := map[string]bool{}
	total := 0
	for _, b := range inputs {
		var check struct {
			SchemaVersion int             `json:"schema_version"`
			ID            string          `json:"check_id"`
			Kind          string          `json:"kind"`
			ObservedAt    time.Time       `json:"observed_at"`
			Status        string          `json:"status"`
			Data          json.RawMessage `json:"data"`
		}
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.DisallowUnknownFields()
		if d.Decode(&check) != nil || check.SchemaVersion != 1 || check.ID == "" || check.ObservedAt.IsZero() || check.ObservedAt.After(a.now().Add(5*time.Minute)) || (check.Kind != "backup" && check.Kind != "payment") || (check.Status != "ok" && check.Status != "error") {
			partial = true
			continue
		}
		if app.ValidateChecks(append(append([]byte{'['}, b...), ']')) != nil {
			partial = true
			continue
		}
		if seen[check.ID] || total+len(b) > 8<<20 {
			partial = true
			continue
		}
		seen[check.ID] = true
		total += len(b)
		rows = append(rows, b)
	}
	b, _ := json.Marshal(rows)
	s := domain.Section{Status: "ok", Complete: true, ObservedAt: a.now(), Data: b}
	if partial {
		s.Status = "partial"
		s.Complete = false
		s.ErrorCode = "invalid_check"
	}
	return s
}
