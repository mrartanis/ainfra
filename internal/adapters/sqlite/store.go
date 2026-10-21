package sqlite

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed migrations/001.sql
var migration string

//go:embed migrations/002.sql
var migration2 string

type Store struct{ db, readDB *sql.DB }
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
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		db.Close()
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	uri.RawQuery = url.Values{"mode": {"ro"}, "_query_only": {"1"}, "_pragma": {"busy_timeout(5000)", "foreign_keys(1)"}}.Encode()
	readers, err := sql.Open("sqlite", uri.String())
	if err != nil {
		db.Close()
		return nil, err
	}
	readers.SetMaxOpenConns(4)
	readers.SetMaxIdleConns(4)
	if err = readers.Ping(); err != nil {
		readers.Close()
		db.Close()
		return nil, err
	}
	return &Store{db: db, readDB: readers}, nil
}
func (s *Store) Close() error { return errors.Join(s.readDB.Close(), s.db.Close()) }
func (s *Store) Read(ctx context.Context, f func(app.Tx) error) error {
	return runTransaction(ctx, s.readDB, f)
}
func (s *Store) Write(ctx context.Context, f func(app.Tx) error) error {
	return runTransaction(ctx, s.db, f)
}
func runTransaction(ctx context.Context, db *sql.DB, f func(app.Tx) error) error {
	tx, e := db.BeginTx(ctx, nil)
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
	projection := "json_object('kind',kind,'id',id,'title',COALESCE(json_extract(body,'$.title'),json_extract(body,'$.name')),'revision',COALESCE(json_extract(body,'$.revision'),0),'node_id',CASE WHEN kind='node' THEN id ELSE json_extract(body,'$.nodes[0]') END,'url',CASE WHEN kind='node' THEN '/nodes/'||id ELSE '/docs/'||id END,'snippet',%s)"
	query := "SELECT " + fmt.Sprintf(projection, "snippet(search,2,'','','…',24)") + " FROM search WHERE search MATCH ? ORDER BY rank,kind,id LIMIT ? OFFSET ?"
	args := []any{`"` + strings.ReplaceAll(q, `"`, `""`) + `"`, limit, offset}
	if literal {
		query = "SELECT " + fmt.Sprintf(projection, "substr(body,MAX(1,instr(lower(body),lower(?))-80),240)") + " FROM records WHERE kind IN ('node','document') AND instr(lower(body),lower(?))>0 ORDER BY kind,id LIMIT ? OFFSET ?"
		args = []any{q, q, limit, offset}
	}
	rows, e := s.readDB.QueryContext(ctx, query, args...)
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

// Serialize migrators on one connection before examining schema state. No
// older binary may silently accept a newer schema.
func migrate(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, e := db.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); e != nil {
		return e
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var version int
	if e = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 2 {
		return fmt.Errorf("database schema %d is newer than supported schema 2", version)
	}
	if version == 0 {
		if _, e = conn.ExecContext(ctx, migration); e != nil {
			return e
		}
		if _, e = conn.ExecContext(ctx, "DELETE FROM search; INSERT INTO search(kind,id,body) SELECT kind,id,body FROM records WHERE kind IN ('node','document'); PRAGMA user_version=1"); e != nil {
			return e
		}
	}
	if version < 2 {
		if _, e = conn.ExecContext(ctx, migration2); e != nil {
			return e
		}
		rows, e := conn.QueryContext(ctx, "SELECT id FROM records WHERE kind='node' ORDER BY id")
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		for _, id := range ids {
			var raw []byte
			if e = conn.QueryRowContext(ctx, "SELECT body FROM records WHERE kind='node' AND id=?", id).Scan(&raw); e != nil {
				return e
			}
			var n domain.Node
			if e = json.Unmarshal(raw, &n); e != nil {
				return e
			}
			body, e := json.Marshal(domain.SummarizeNode(n))
			if e != nil {
				return e
			}
			if _, e = conn.ExecContext(ctx, "INSERT INTO records(kind,id,body) VALUES('node-summary',?,?) ON CONFLICT(kind,id) DO UPDATE SET body=excluded.body", id, string(body)); e != nil {
				return e
			}
		}
		if _, e = conn.ExecContext(ctx, "PRAGMA user_version=2"); e != nil {
			return e
		}
	}
	_, e = conn.ExecContext(ctx, "COMMIT")
	return e
}

func (t transaction) ListNode(kind, node string) ([]json.RawMessage, error) {
	rows, e := t.tx.QueryContext(t.ctx, "SELECT body FROM records WHERE kind=? AND json_extract(body,'$.node_id')=? ORDER BY id", kind, node)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(raw))
	}
	return out, rows.Err()
}

func (s *Store) CountSearch(ctx context.Context, q string, literal bool) (count int, err error) {
	query := "SELECT count(*) FROM search WHERE search MATCH ?"
	if literal {
		query = "SELECT count(*) FROM records WHERE kind IN ('node','document') AND instr(lower(body),lower(?))>0"
	} else {
		q = `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	}
	err = s.readDB.QueryRowContext(ctx, query, q).Scan(&count)
	return
}

func (t transaction) TopologyNodes() ([]domain.TopologyNode, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT json_object('id',id,'name',json_extract(body,'$.name'),'lifecycle',json_extract(body,'$.lifecycle'),'addresses',json_extract(body,'$.addresses'),'wireguard_public_keys',json_extract(body,'$.wireguard_public_keys'),'network',json_extract(body,'$.observations.network'),'wireguard',json_extract(body,'$.observations.wireguard')) FROM records WHERE kind='node' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TopologyNode{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var n domain.TopologyNode
		if err = json.Unmarshal(raw, &n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
