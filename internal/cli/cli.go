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
	"strconv"
	"strings"
)

const Help = `infractl — клиент HTTP API Infra
  nodes list | nodes show ID | nodes put --file node.json --if-revision N
  attention | packages list NODE [--upgradable] | findings list [--node ID]
  docs search QUERY [--literal] | docs get ID [--revision N]
  docs put --file document.json [--if-revision N]
  docs approve|complete|cancel ID --if-revision N
  relations list | relations suggest [--node ID] | relations put --file relation.json | relations delete ID
  tokens list | tokens issue --file principal.json --output token-file
  tokens revoke ID | feeds status | events | me
  context NODE [--format markdown] | export --output NEW_DIRECTORY
  request METHOD API_PATH [--file JSON] [--if-revision N]
  --limit N --offset N: страница списка; --page-info: данные с метаданными страницы.
  --json: машинный JSON; --config: JSON с endpoint, token_file, ca_file.
  Формат docs put: JSON {id?, title, kind, body, nodes?, tags?}; без id создаёт документ.
`

func Run(ctx context.Context, c *apiclient.Client, args []string, out io.Writer) error {
	opts := map[string]string{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			k := strings.TrimPrefix(a, "--")
			switch k {
			case "json", "literal", "upgradable", "help", "page-info":
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
		case "complete", "cancel":
			path = "documents/" + url.PathEscape(at(2)) + "/status"
			method = "POST"
			status := "completed"
			if at(1) == "cancel" {
				status = "cancelled"
			}
			input = map[string]any{"status": status}
		case "approve":
			path = "documents/" + url.PathEscape(at(2)) + "/approve"
			method = "POST"
		}
	case "relations":
		path = "relations"
		if at(1) == "suggest" {
			path = "relation-suggestions?node=" + url.QueryEscape(opts["node"])
		}
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
		return exportAPI(ctx, c, opts["output"])
	case "request":
		method = at(1)
		path = at(2)
	}
	if path == "" || strings.Contains(path, "..") || strings.Contains(path, "://") {
		return errors.New("invalid command; use --help")
	}
	if method == "GET" {
		u, err := url.Parse(path)
		if err != nil {
			return err
		}
		q := u.Query()
		for _, key := range []string{"limit", "offset"} {
			if opts[key] != "" {
				q.Set(key, opts[key])
			}
		}
		u.RawQuery = q.Encode()
		path = u.String()
	}
	var result any
	var page apiclient.Page
	if e = c.Do(ctx, method, path, input, &result, rev, &page); e != nil {
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
	if opts["page-info"] == "true" {
		result = map[string]any{"items": result, "pagination": page}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
