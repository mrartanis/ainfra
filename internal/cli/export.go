package cli

import (
	"ainfra/internal/apiclient"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func exportAPI(ctx context.Context, c *apiclient.Client, target string) error {
	if target == "" {
		return errors.New("--output required")
	}
	if _, e := os.Stat(target); !os.IsNotExist(e) {
		return errors.New("export destination exists")
	}
	if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(target), ".infra-export-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	start := time.Now().UTC()
	write := func(path string, b []byte) error {
		p := filepath.Join(tmp, path)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		return os.WriteFile(p, b, 0600)
	}
	safe := func(s string) bool { return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\") }
	if e = c.ForEach(ctx, "nodes", func(raw json.RawMessage) error {
		var node struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		}
		if e := json.Unmarshal(raw, &node); e != nil {
			return e
		}
		if !safe(node.Slug) || !safe(node.ID) {
			return errors.New("unsafe node identity")
		}
		var full json.RawMessage
		if e := c.Do(ctx, "GET", "nodes/"+url.PathEscape(node.ID), nil, &full, 0); e != nil {
			return e
		}
		return write("nodes/"+node.Slug+"/node.json", full)
	}); e != nil {
		return e
	}
	if e = c.ForEach(ctx, "documents", func(raw json.RawMessage) error {
		var doc struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if e := json.Unmarshal(raw, &doc); e != nil {
			return e
		}
		if !safe(doc.ID) {
			return errors.New("unsafe document identity")
		}
		if e := write("documents/"+doc.ID+".md", []byte("# "+doc.Title+"\n\n"+doc.Body)); e != nil {
			return e
		}
		return write("documents/"+doc.ID+".json", raw)
	}); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(tmp, "history"), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(tmp, "history/events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	e = c.ForEach(ctx, "events", func(raw json.RawMessage) error { _, e := f.Write(append(raw, '\n')); return e })
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	relations := []json.RawMessage{}
	if e = c.ForEach(ctx, "relations", func(raw json.RawMessage) error { relations = append(relations, raw); return nil }); e != nil {
		return e
	}
	b, _ := json.Marshal(relations)
	if e = write("relations.json", b); e != nil {
		return e
	}
	manifest, _ := json.Marshal(map[string]any{"schema_version": 1, "contains_secrets": false, "restore_requires_database_backup": true, "started_at": start, "finished_at": time.Now().UTC(), "consistency": "per-record revisions; not an atomic database snapshot"})
	if e = write("manifest.json", manifest); e != nil {
		return e
	}
	return os.Rename(tmp, target)
}
