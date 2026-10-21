package cli

import (
	"ainfra/internal/apiclient"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const Help = `infractl — клиент HTTP API Infra
  nodes list | nodes show ID | nodes put --file node.json --if-revision N
  attention | packages list NODE [--upgradable] | findings list [--node ID]
  docs search QUERY [--literal] | docs get ID [--revision N]
  docs put --file document.json [--if-revision N]
  docs approve ID --if-revision N
  relations list | relations put --file relation.json | relations delete ID
  tokens list | tokens issue --file principal.json --output token-file
  tokens revoke ID | feeds status | events | me
  context NODE [--format markdown] | export --output NEW_DIRECTORY
  request METHOD API_PATH [--file JSON] [--if-revision N]
  --json: машинный JSON; --config: JSON с endpoint, token_file, ca_file.
  Формат docs put: JSON {id?, title, kind, body, nodes?}; без id создаёт документ.
`

func Run(ctx context.Context, c *apiclient.Client, args []string, out io.Writer) error {
	opts := map[string]string{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			k := strings.TrimPrefix(a, "--")
			switch k {
			case "json", "literal", "upgradable", "help":
				opts[k] = "true"
			default:
				if i+1 >= len(args) {
					return fmt.Errorf("missing %s value", a)
				}
				i++
				opts[k] = args[i]
			}
		} else {
			pos = append(pos, a)
		}
	}
	if len(pos) == 0 || opts["help"] == "true" {
		_, e := io.WriteString(out, Help)
		return e
	}
	rev, _ := strconv.Atoi(opts["if-revision"])
	path := ""
	method := "GET"
	var input any
	var e error
	if file := opts["file"]; file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if e = json.Unmarshal(b, &input); e != nil {
			return e
		}
	}
	at := func(i int) string {
		if len(pos) > i {
			return pos[i]
		}
		return ""
	}
	switch pos[0] {
	case "nodes":
		switch at(1) {
		case "list":
			path = "nodes"
		case "show":
			path = "nodes/" + url.PathEscape(at(2))
		case "put":
			path = "nodes"
			method = "POST"
			if m, ok := input.(map[string]any); ok {
				if id, ok := m["id"].(string); ok && id != "" {
					path += "/" + url.PathEscape(id)
					method = "PATCH"
				}
			}
		}
	case "attention", "events", "me":
		path = pos[0]
	case "packages":
		if at(1) == "list" {
			path = "nodes/" + url.PathEscape(at(2)) + "/packages?upgradable=" + opts["upgradable"]
		}
	case "findings":
		if at(1) == "list" {
			path = "findings?node=" + url.QueryEscape(opts["node"])
		}
	case "feeds":
		if at(1) == "status" {
			path = "feed-status"
		}
	case "docs":
		switch at(1) {
		case "search":
			path = "search?q=" + url.QueryEscape(at(2)) + "&literal=" + opts["literal"]
		case "get":
			path = "documents/" + url.PathEscape(at(2)) + "?revision=" + opts["revision"]
		case "put":
			path = "documents"
			method = "POST"
			if m, ok := input.(map[string]any); ok {
				if id, ok := m["id"].(string); ok && id != "" {
					path += "/" + url.PathEscape(id)
					method = "PATCH"
				}
			}
		case "approve":
			path = "documents/" + url.PathEscape(at(2)) + "/approve"
			method = "POST"
		}
	case "relations":
		path = "relations"
		if at(1) == "put" {
			method = "POST"
		}
		if at(1) == "delete" {
			method = "DELETE"
			path += "/" + url.PathEscape(at(2))
		}
	case "tokens":
		path = "tokens"
		if at(1) == "issue" {
			method = "POST"
			if opts["output"] == "" {
				return errors.New("--output private token file required")
			}
		}
		if at(1) == "revoke" {
			method = "DELETE"
			path += "/" + url.PathEscape(at(2))
		}
	case "context":
		var text string
		e = c.Do(ctx, "GET", "nodes/"+url.PathEscape(at(1))+"/context", nil, &text, 0)
		if e != nil {
			return e
		}
		_, e = io.WriteString(out, text)
		return e
	case "export":
		var data map[string]json.RawMessage
		if e = c.Do(ctx, "GET", "export", nil, &data, 0); e != nil {
			return e
		}
		return export(opts["output"], data)
	case "request":
		method = at(1)
		path = at(2)
	}
	if path == "" || strings.Contains(path, "..") || strings.Contains(path, "://") {
		return errors.New("invalid command; use --help")
	}
	var result any
	if e = c.Do(ctx, method, path, input, &result, rev); e != nil {
		return e
	}
	if pos[0] == "tokens" && at(1) == "issue" {
		m, ok := result.(map[string]any)
		if !ok {
			return errors.New("invalid token response")
		}
		secret, ok := m["secret"].(string)
		if !ok {
			return errors.New("missing token")
		}
		f, e := os.OpenFile(opts["output"], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = io.WriteString(f, secret+"\n")
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		result = m["token"]
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
func export(target string, data map[string]json.RawMessage) error {
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
	write := func(path string, b []byte) error {
		p := filepath.Join(tmp, path)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		return os.WriteFile(p, b, 0600)
	}
	safe := func(s string) bool { return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\") }
	var nodes []map[string]any
	if e = json.Unmarshal(data["nodes"], &nodes); e != nil {
		return e
	}
	for _, n := range nodes {
		id, _ := n["slug"].(string)
		if !safe(id) {
			return errors.New("unsafe node slug")
		}
		b, _ := json.MarshalIndent(n, "", "  ")
		if e = write("nodes/"+id+"/node.json", b); e != nil {
			return e
		}
	}
	var docs []struct {
		ID    string `json:"id"`
		Body  string `json:"body"`
		Title string `json:"title"`
	}
	if e = json.Unmarshal(data["documents"], &docs); e != nil {
		return e
	}
	for _, d := range docs {
		if !safe(d.ID) {
			return errors.New("unsafe document ID")
		}
		if e = write("documents/"+d.ID+".md", []byte("# "+d.Title+"\n\n"+d.Body)); e != nil {
			return e
		}
	}
	var events []json.RawMessage
	if e = json.Unmarshal(data["events"], &events); e != nil {
		return e
	}
	var b strings.Builder
	for _, v := range events {
		b.Write(v)
		b.WriteByte('\n')
	}
	if e = write("history/events.jsonl", []byte(b.String())); e != nil {
		return e
	}
	if e = write("relations.json", data["relations"]); e != nil {
		return e
	}
	if e = write("manifest.json", []byte(`{"schema_version":1,"contains_secrets":false,"restore_requires_database_backup":true}`)); e != nil {
		return e
	}
	return os.Rename(tmp, target)
}
