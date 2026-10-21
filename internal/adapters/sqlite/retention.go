package sqlite

import (
	"ainfra/internal/app"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Retention struct {
	Days              int `json:"days"`
	MaxHistory        int `json:"max_history"`
	DocumentRevisions int `json:"document_revisions"`
	MaxDatabaseMiB    int `json:"max_database_mib"`
}
type RetentionResult struct {
	Policy        Retention `json:"policy"`
	LastRun       time.Time `json:"last_run"`
	Deleted       int64     `json:"deleted"`
	DatabaseBytes int64     `json:"database_bytes"`
	FreeBytes     int64     `json:"reusable_bytes"`
}

func (s *Store) SetSizeLimit(ctx context.Context, mib int) error {
	if mib < 64 || mib > 65536 {
		return app.ErrInvalid
	}
	var page int64
	if e := s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&page); e != nil {
		return e
	}
	requested := int64(mib) * (1 << 20) / page
	var actual int64
	if e := s.db.QueryRowContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", requested)).Scan(&actual); e != nil {
		return e
	}
	if actual > requested {
		return fmt.Errorf("existing database exceeds configured size limit")
	}
	_, e := s.db.ExecContext(ctx, "PRAGMA journal_size_limit=16777216; PRAGMA wal_autocheckpoint=256")
	return e
}
func (s *Store) Prune(ctx context.Context, p Retention, now time.Time) (out RetentionResult, err error) {
	out = RetentionResult{Policy: p, LastRun: now}
	if p.Days < 1 || p.MaxHistory < 1 || p.DocumentRevisions < 1 {
		return out, app.ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	cutoff := now.AddDate(0, 0, -p.Days).Format(time.RFC3339Nano)
	for _, kind := range []string{"receipt", "event"} {
		field := "$.at"
		if kind == "receipt" {
			field = "$.received_at"
		}
		res, e := tx.ExecContext(ctx, `DELETE FROM records WHERE kind=? AND (julianday(json_extract(body,?))<julianday(?) OR id IN (SELECT id FROM records WHERE kind=? ORDER BY julianday(json_extract(body,?)) DESC,id DESC LIMIT -1 OFFSET ?))`, kind, field, cutoff, kind, field, p.MaxHistory)
		if e != nil {
			return out, e
		}
		n, _ := res.RowsAffected()
		out.Deleted += n
	}
	res, e := tx.ExecContext(ctx, `DELETE FROM records WHERE kind='revision' AND (kind,id) IN (SELECT kind,id FROM (SELECT kind,id,body,ROW_NUMBER() OVER (PARTITION BY json_extract(body,'$.id') ORDER BY CAST(json_extract(body,'$.revision') AS INTEGER) DESC) AS rn FROM records WHERE kind='revision') WHERE rn>?) AND NOT EXISTS (SELECT 1 FROM records AS current WHERE current.kind='document' AND current.id=json_extract(records.body,'$.id') AND (json_extract(current.body,'$.approved_revision')=json_extract(records.body,'$.revision') OR json_extract(current.body,'$.revision')=json_extract(records.body,'$.revision')))`, p.DocumentRevisions)
	if e != nil {
		return out, e
	}
	n, _ := res.RowsAffected()
	out.Deleted += n
	res, e = tx.ExecContext(ctx, `DELETE FROM records WHERE kind='finding' AND json_extract(body,'$.workflow')='resolved' AND julianday(json_extract(body,'$.last_evaluated'))<julianday(?)`, cutoff)
	if e != nil {
		return out, e
	}
	n, _ = res.RowsAffected()
	out.Deleted += n
	var pages, pageSize, free int64
	for _, q := range []struct {
		sql  string
		dest *int64
	}{{"PRAGMA page_count", &pages}, {"PRAGMA page_size", &pageSize}, {"PRAGMA freelist_count", &free}} {
		if e = tx.QueryRowContext(ctx, q.sql).Scan(q.dest); e != nil {
			return out, e
		}
	}
	out.DatabaseBytes = pages * pageSize
	out.FreeBytes = free * pageSize
	b, e := json.Marshal(out)
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO records(kind,id,body) VALUES('settings','retention',?) ON CONFLICT(kind,id) DO UPDATE SET body=excluded.body`, string(b)); e != nil {
		return out, e
	}
	if e = tx.Commit(); e != nil {
		return out, e
	}
	_, e = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return out, e
}
