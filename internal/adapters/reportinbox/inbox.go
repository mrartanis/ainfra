// Package reportinbox durably accepts reports independently of inventory writes.
package reportinbox

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const MaxReports = 1000
const MaxBytes = 256 << 20

type Inbox struct {
	db   *sql.DB
	lock *os.File
	wake chan struct{}
	mu   sync.Mutex
}
type job struct {
	ID        string
	Principal domain.Principal
	Report    domain.Report
}

func Open(path string) (*Inbox, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		lock.Close()
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*Inbox, error) { db.Close(); lock.Close(); return nil, e }
	for _, q := range []string{
		"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL",
		`CREATE TABLE IF NOT EXISTS agent_inbox (
   report_id TEXT PRIMARY KEY, node_id TEXT NOT NULL, content_hash TEXT NOT NULL,
   principal BLOB NOT NULL, report BLOB NOT NULL, received_at TEXT NOT NULL,
   state TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0,
   retry_after INTEGER NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '')`,
		"CREATE INDEX IF NOT EXISTS agent_inbox_pending ON agent_inbox(state,retry_after,received_at)",
		"UPDATE agent_inbox SET state='pending',retry_after=0 WHERE state='processing'",
	} {
		if _, err = db.Exec(q); err != nil {
			return fail(err)
		}
	}
	return &Inbox{db: db, lock: lock, wake: make(chan struct{}, 1)}, nil
}
func (q *Inbox) Close() error { e := q.db.Close(); return errors.Join(e, q.lock.Close()) }

func (q *Inbox) Enqueue(ctx context.Context, p domain.Principal, r domain.Report) (domain.Ack, error) {
	ack := domain.Ack{ID: r.ID, Status: "queued"}
	b, err := json.Marshal(r)
	if err != nil {
		return ack, err
	}
	if len(b) > 16<<20 {
		return ack, app.ErrInvalid
	}
	principal, err := json.Marshal(p)
	if err != nil {
		return ack, err
	}
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	q.mu.Lock()
	defer q.mu.Unlock()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return ack, app.ErrUnavailable
	}
	defer tx.Rollback()
	var oldHash string
	err = tx.QueryRowContext(ctx, "SELECT content_hash FROM agent_inbox WHERE report_id=?", r.ID).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return ack, app.ErrConflict
		}
		ack.Status = "duplicate"
		return ack, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ack, app.ErrUnavailable
	}
	var count int
	var bytes int64
	if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(length(report)+length(principal)),0) FROM agent_inbox").Scan(&count, &bytes); err != nil {
		return ack, app.ErrUnavailable
	}
	if count >= MaxReports || bytes+int64(len(b)+len(principal)) > MaxBytes {
		return ack, app.ErrUnavailable
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO agent_inbox(report_id,node_id,content_hash,principal,report,received_at) VALUES(?,?,?,?,?,?)", r.ID, r.NodeID, hash, principal, b, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return ack, app.ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return ack, app.ErrUnavailable
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return ack, nil
}

func (q *Inbox) Status(ctx context.Context) (domain.InboxStatus, error) {
	out := domain.InboxStatus{Reports: []domain.InboxReport{}}
	err := q.db.QueryRowContext(ctx, `SELECT coalesce(sum(state='pending'),0),coalesce(sum(state='processing'),0),coalesce(sum(state='rejected'),0),coalesce(sum(length(report)+length(principal)),0) FROM agent_inbox`).Scan(&out.Pending, &out.Processing, &out.Rejected, &out.Bytes)
	if err != nil {
		return out, err
	}
	rows, err := q.db.QueryContext(ctx, "SELECT report_id,node_id,state,attempts,received_at,error_code FROM agent_inbox ORDER BY received_at,report_id LIMIT 100")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.InboxReport
		var date string
		if err = rows.Scan(&r.ID, &r.NodeID, &r.State, &r.Attempts, &date, &r.ErrorCode); err != nil {
			return out, err
		}
		r.ReceivedAt, _ = time.Parse(time.RFC3339Nano, date)
		out.Reports = append(out.Reports, r)
	}
	return out, rows.Err()
}

func (q *Inbox) take(ctx context.Context) (job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var j job
	var p, b []byte
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return j, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "SELECT report_id,principal,report FROM agent_inbox WHERE state='pending' AND retry_after<=? ORDER BY received_at,report_id LIMIT 1", time.Now().Unix()).Scan(&j.ID, &p, &b)
	if err != nil {
		return j, err
	}
	if err = json.Unmarshal(p, &j.Principal); err != nil {
		return j, err
	}
	if err = json.Unmarshal(b, &j.Report); err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE agent_inbox SET state='processing',attempts=attempts+1 WHERE report_id=?", j.ID); err != nil {
		return j, err
	}
	return j, tx.Commit()
}

// ProcessOne commits inventory before removing the payload. A crash between those
// commits is safe: inventory receipt hashes and sequence checks make replay idempotent.
func (q *Inbox) ProcessOne(ctx context.Context, handle func(context.Context, domain.Principal, domain.Report) (domain.Ack, error)) (bool, error) {
	j, err := q.take(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	_, err = handle(run, j.Principal, j.Report)
	cancel()
	finish, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err == nil {
		_, err = q.db.ExecContext(finish, "DELETE FROM agent_inbox WHERE report_id=?", j.ID)
		return true, err
	}
	state, code := "pending", "processing_failed"
	switch {
	case errors.Is(err, app.ErrConflict):
		state, code = "rejected", "conflict"
	case errors.Is(err, app.ErrForbidden):
		state, code = "rejected", "forbidden"
	case errors.Is(err, app.ErrNotFound):
		state, code = "rejected", "not_found"
	case errors.Is(err, app.ErrInvalid):
		state, code = "rejected", "invalid_input"
	}
	_, saveErr := q.db.ExecContext(finish, "UPDATE agent_inbox SET state=?,error_code=?,retry_after=? WHERE report_id=?", state, code, time.Now().Add(30*time.Second).Unix(), j.ID)
	return true, errors.Join(err, saveErr)
}

func (q *Inbox) Run(ctx context.Context, handle func(context.Context, domain.Principal, domain.Report) (domain.Ack, error), logger *slog.Logger) {
	for ctx.Err() == nil {
		worked, err := q.ProcessOne(ctx, handle)
		if err != nil && ctx.Err() == nil && logger != nil {
			logger.Error("inbox processing failed; report retained")
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-time.After(time.Second):
		}
	}
}
