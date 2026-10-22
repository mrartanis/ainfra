package reportinbox

import (
	"ainfra/internal/adapters/httpapi"
	"ainfra/internal/adapters/sqlite"
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func report() (domain.Principal, domain.Report) {
	p := domain.Principal{Role: "agent", Nodes: []string{"node"}, Instance: "instance"}
	r := domain.Report{SchemaVersion: 1, ID: "report-1", NodeID: "node", Instance: "instance", Sequence: 1, ObservedAt: time.Now(), Collectors: map[string]domain.Section{"os": {Status: "ok", Complete: true, ObservedAt: time.Now(), Data: json.RawMessage(`{"id":"debian","codename":"bookworm"}`)}}}
	return p, r
}
func TestDurabilityDuplicateConflictAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	q, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, r := report()
	ack, err := q.Enqueue(ctx, p, r)
	if err != nil || ack.Status != "queued" {
		t.Fatal(ack, err)
	}
	ack, err = q.Enqueue(ctx, p, r)
	if err != nil || ack.Status != "duplicate" {
		t.Fatal(ack, err)
	}
	changed := r
	changed.Sequence++
	if _, err = q.Enqueue(ctx, p, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	called := false
	worked, err := q.ProcessOne(ctx, func(_ context.Context, actual domain.Principal, received domain.Report) (domain.Ack, error) {
		called = true
		if actual.Instance != p.Instance || received.ID != r.ID {
			t.Fatal(actual, received)
		}
		return domain.Ack{ID: r.ID, Status: "accepted"}, nil
	})
	if err != nil || !worked || !called {
		t.Fatal(worked, called, err)
	}
	status, err := q.Status(ctx)
	if err != nil || status.Pending+status.Processing+status.Rejected != 0 {
		t.Fatal(status, err)
	}
}

func TestRestartAfterInventoryCommitReplaysIdempotently(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := app.New(store, time.Now, app.RandomID)
	admin := domain.Principal{Role: "admin"}
	node, err := service.SaveNode(ctx, admin, domain.Node{Name: "host"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, r := report()
	p.Nodes = []string{node.ID}
	r.NodeID = node.ID
	path := filepath.Join(dir, "inbox.db")
	q, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.Enqueue(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	_, err = q.ProcessOne(ctx, func(ctx context.Context, p domain.Principal, r domain.Report) (domain.Ack, error) {
		ack, err := service.IngestReport(ctx, p, r)
		if err != nil {
			return ack, err
		}
		// Simulate loss of the inbox connection after the inventory transaction commits.
		if err = q.Close(); err != nil {
			t.Fatal(err)
		}
		return ack, nil
	})
	if err == nil {
		t.Fatal("expected interrupted inbox cleanup")
	}
	q, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	worked, err := q.ProcessOne(ctx, func(ctx context.Context, p domain.Principal, r domain.Report) (domain.Ack, error) {
		ack, err := service.IngestReport(ctx, p, r)
		if ack.Status != "duplicate" {
			t.Fatal(ack)
		}
		return ack, err
	})
	if err != nil || !worked {
		t.Fatal(worked, err)
	}
	n, err := service.Node(ctx, admin, node.ID)
	if err != nil || n.Sequence != 1 {
		t.Fatal(n.Sequence, err)
	}
	status, err := q.Status(ctx)
	if err != nil || status.Pending+status.Processing != 0 {
		t.Fatal(status, err)
	}
}

func TestRejectedAndTransientFailuresRetainPayload(t *testing.T) {
	ctx := context.Background()
	q, err := Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	p, r := report()
	q.Enqueue(ctx, p, r)
	q.ProcessOne(ctx, func(context.Context, domain.Principal, domain.Report) (domain.Ack, error) {
		return domain.Ack{}, errors.New("sensitive internal detail")
	})
	status, err := q.Status(ctx)
	if err != nil || status.Pending != 1 || status.Reports[0].ErrorCode != "processing_failed" {
		t.Fatal(status, err)
	}
	q.db.Exec("UPDATE agent_inbox SET retry_after=0")
	q.ProcessOne(ctx, func(context.Context, domain.Principal, domain.Report) (domain.Ack, error) {
		return domain.Ack{}, app.ErrConflict
	})
	status, err = q.Status(ctx)
	if err != nil || status.Rejected != 1 || status.Reports[0].ErrorCode != "conflict" || status.Bytes == 0 {
		t.Fatal(status, err)
	}
}

func TestOnlineBackupRestoresAcceptedReports(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path, copy := filepath.Join(dir, "live.db"), filepath.Join(dir, "copy.db")
	q, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	p, r := report()
	if _, err = q.Enqueue(ctx, p, r); err != nil {
		t.Fatal(err)
	}
	if err = Backup(ctx, path, copy); err != nil {
		t.Fatal(err)
	}
	if err = Backup(ctx, path, copy); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	restored, err := Open(copy)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	status, err := restored.Status(ctx)
	if err != nil || status.Pending != 1 {
		t.Fatal(status, err)
	}
	st, err := os.Stat(copy)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal(st, err)
	}
}

func TestCapacityRefusesNewPayloadWithoutLosingAcceptedReports(t *testing.T) {
	ctx := context.Background()
	q, err := Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	tx, err := q.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxReports; i++ {
		if _, err = tx.Exec("INSERT INTO agent_inbox(report_id,node_id,content_hash,principal,report,received_at) VALUES(?,?,?,?,?,?)", fmt.Sprint(i), "n", "h", []byte(`{}`), []byte(`{}`), time.Now().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	p, r := report()
	if _, err = q.Enqueue(ctx, p, r); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	status, err := q.Status(ctx)
	if err != nil || status.Pending != MaxReports {
		t.Fatal(status.Pending, err)
	}
}

func TestHTTPReceiptIndependentOfInventoryWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "inventory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := app.New(store, time.Now, app.RandomID)
	admin := domain.Principal{Role: "admin"}
	node, err := service.SaveNode(ctx, admin, domain.Node{Name: "host"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, r := report()
	p.Nodes = []string{node.ID}
	r.NodeID = node.ID
	token, _, err := service.IssueToken(ctx, admin, p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := Open(filepath.Join(dir, "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	handler := httpapi.NewWithInbox(service, nil, q)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- store.Write(ctx, func(tx app.Tx) error {
			if err := tx.Put("settings", "held", map[string]bool{"busy": true}); err != nil {
				return err
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	defer func() {
		close(release)
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	b, _ := json.Marshal(r)
	req := httptest.NewRequest("POST", "/api/v1/ingest/reports", strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+token)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { w := httptest.NewRecorder(); handler.ServeHTTP(w, req); done <- w }()
	select {
	case w := <-done:
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"queued"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("receipt waited for the inventory write transaction")
	}
	n, err := service.Node(ctx, admin, node.ID)
	if err != nil || n.Sequence != 0 {
		t.Fatal(n.Sequence, err)
	}
	status, err := q.Status(ctx)
	if err != nil || status.Pending != 1 {
		t.Fatal(status, err)
	}
	// Agents retain their write-only restriction on queue status.
	req = httptest.NewRequest("GET", "/api/v1/ingest/queue", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
