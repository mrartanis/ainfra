package agent_test

import (
	"ainfra/internal/agent"
	"ainfra/internal/domain"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

type collector struct{}

func (collector) Collect(context.Context, []string) map[string]domain.Section {
	return map[string]domain.Section{}
}

type transport struct {
	fail bool
	ids  []string
}

func (t *transport) Send(ctx context.Context, r domain.Report) error {
	t.ids = append(t.ids, r.ID)
	if t.fail {
		return errors.New("offline")
	}
	return nil
}
func TestRetryThenDiscardAndNextRunCollectsFresh(t *testing.T) {
	dir := t.TempDir()
	tx := &transport{fail: true}
	n := 0
	a := agent.New(agent.Config{NodeID: "node", Instance: "instance", StateDir: dir, Collectors: []string{"os"}}, collector{}, tx, time.Now, func() string { n++; return string(rune('a' + n)) })
	a.SetRetryWait(func(context.Context, time.Duration) error { return nil })
	if e := a.Run(context.Background()); e == nil {
		t.Fatal("offline succeeded")
	}
	if len(tx.ids) != 3 || tx.ids[0] != tx.ids[1] || tx.ids[1] != tx.ids[2] {
		t.Fatal(tx.ids)
	}
	tx.fail = false
	if e := a.Run(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(tx.ids) != 4 || tx.ids[3] == tx.ids[0] {
		t.Fatal("old report redelivered", tx.ids)
	}
	if _, e := os.Stat(dir + "/spool"); !os.IsNotExist(e) {
		t.Fatal("spool should not exist")
	}
	b, _ := os.ReadFile(dir + "/sequence")
	if string(b) != "2" {
		t.Fatalf("sequence %s", b)
	}
}
