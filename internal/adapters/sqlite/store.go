package sqlite

import (
	"ainfra/internal/app"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
)

//go:embed migrations/001.sql
var migration string

type Store struct{ db *sql.DB }
type transaction struct {
	tx  *sql.Tx
	ctx context.Context
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", migration} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db}, nil
}
func (s *Store) Close() error                                          { return s.db.Close() }
func (s *Store) Read(ctx context.Context, f func(app.Tx) error) error  { return s.run(ctx, f) }
func (s *Store) Write(ctx context.Context, f func(app.Tx) error) error { return s.run(ctx, f) }
func (s *Store) run(ctx context.Context, f func(app.Tx) error) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = f(transaction{tx, ctx}); e != nil {
		return e
	}
	return tx.Commit()
}
func (t transaction) Get(k, id string, out any) error {
	var b []byte
	e := t.tx.QueryRowContext(t.ctx, "SELECT body FROM records WHERE kind=? AND id=?", k, id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func (t transaction) Put(k, id string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = t.tx.ExecContext(t.ctx, "INSERT INTO records(kind,id,body) VALUES(?,?,?) ON CONFLICT(kind,id) DO UPDATE SET body=excluded.body", k, id, string(b))
	return e
}
func (t transaction) Delete(k, id string) error {
	_, e := t.tx.ExecContext(t.ctx, "DELETE FROM records WHERE kind=? AND id=?", k, id)
	return e
}
func (t transaction) List(k string) ([]json.RawMessage, error) {
	rows, e := t.tx.QueryContext(t.ctx, "SELECT body FROM records WHERE kind=? ORDER BY id", k)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) Search(ctx context.Context, q string, literal bool, limit, offset int) ([]json.RawMessage, error) {
	query := "SELECT body FROM search WHERE search MATCH ? ORDER BY rank LIMIT ? OFFSET ?"
	if literal {
		query = "SELECT body FROM records WHERE kind IN ('node','document') AND instr(lower(body),lower(?))>0 ORDER BY kind,id LIMIT ? OFFSET ?"
	} else {
		q = `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	}
	rows, e := s.db.QueryContext(ctx, query, q, limit, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Store) Backup(ctx context.Context, path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return app.ErrConflict
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
	if err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}
