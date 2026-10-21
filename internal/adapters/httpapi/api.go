package httpapi

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const MaxBody = 16 << 20

type API struct{ s *app.Service }

func New(s *app.Service) http.Handler { return &API{s} }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, e error) {
	status := 500
	code := "internal_error"
	switch {
	case errors.Is(e, app.ErrForbidden):
		status = 403
		code = "forbidden"
	case errors.Is(e, app.ErrNotFound):
		status = 404
		code = "not_found"
	case errors.Is(e, app.ErrConflict):
		status = 409
		code = "conflict"
	case errors.Is(e, app.ErrInvalid):
		status = 400
		code = "invalid_input"
	}
	write(w, status, map[string]string{"error": code})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Header.Get("Content-Encoding") != "" {
		return app.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			write(w, 413, map[string]string{"error": "body_too_large"})
			return e
		}
		return app.ErrInvalid
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return app.ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return app.ErrInvalid
	}
	return nil
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", app.RandomID())
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		write(w, 200, map[string]string{"status": "ok"})
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		write(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	p, e := a.s.Authenticate(ctx, strings.TrimPrefix(auth, "Bearer "))
	if e != nil {
		write(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	parts := strings.Split(path, "/")
	if p.Role == "agent" || p.Role == "probe" {
		if path != "ingest/reports" || r.Method != "POST" {
			failure(w, app.ErrForbidden)
			return
		}
	}
	var out any
	status := 200
	revision, _ := strconv.Atoi(strings.Trim(r.Header.Get("If-Match"), `"`))
	read := r.Method == "GET"
	id := ""
	if len(parts) > 1 {
		id = parts[1]
	}
	body := func(v any) bool {
		e = decode(w, r, v)
		if e != nil {
			var max *http.MaxBytesError
			if !errors.As(e, &max) {
				failure(w, e)
			}
			return false
		}
		return true
	}
	switch {
	case path == "settings" && read:
		out, e = a.s.Settings(ctx, p)
	case path == "me" && read:
		out = p
	case path == "ingest/reports" && r.Method == "POST":
		var v domain.Report
		if !body(&v) {
			return
		}
		out, e = a.s.IngestReport(ctx, p, v)
	case path == "nodes" && read:
		out, e = a.s.Nodes(ctx, p)
	case path == "nodes" && r.Method == "POST":
		var v domain.Node
		if !body(&v) {
			return
		}
		out, e = a.s.SaveNode(ctx, p, v, 0)
		status = 201
	case parts[0] == "nodes" && len(parts) == 2 && read:
		out, e = a.s.Node(ctx, p, id)
	case parts[0] == "nodes" && len(parts) == 2 && r.Method == "PATCH":
		var v domain.Node
		if !body(&v) {
			return
		}
		v.ID = id
		out, e = a.s.SaveNode(ctx, p, v, revision)
	case parts[0] == "nodes" && len(parts) == 3 && parts[2] == "packages" && read:
		out, e = a.s.Packages(ctx, p, id, r.URL.Query().Get("upgradable") == "true")
	case parts[0] == "nodes" && len(parts) == 3 && parts[2] == "findings" && read:
		out, e = a.s.Findings(ctx, p, id)
	case parts[0] == "nodes" && len(parts) == 3 && parts[2] == "context" && read:
		var text string
		text, e = a.s.Context(ctx, p, id)
		if e == nil {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = io.WriteString(w, text)
			return
		}
	case parts[0] == "nodes" && len(parts) == 3 && parts[2] == "pay" && r.Method == "POST":
		out, e = a.s.Pay(ctx, p, id, revision)
	case parts[0] == "nodes" && len(parts) == 3 && parts[2] == "reenroll" && r.Method == "POST":
		var v struct {
			Instance string `json:"agent_instance_id"`
		}
		if !body(&v) {
			return
		}
		e = a.s.Reenroll(ctx, p, id, v.Instance, revision)
		out = map[string]bool{"ok": e == nil}
	case path == "attention" && read:
		out, e = a.s.Attention(ctx, p)
	case path == "findings" && read:
		out, e = a.s.Findings(ctx, p, r.URL.Query().Get("node"))
	case parts[0] == "findings" && len(parts) == 3 && parts[2] == "suppress" && r.Method == "POST":
		var v struct {
			Reason string    `json:"reason"`
			Until  time.Time `json:"until"`
		}
		if !body(&v) {
			return
		}
		e = a.s.Suppress(ctx, p, id, v.Reason, v.Until)
		out = map[string]bool{"ok": e == nil}
	case path == "feed-status" && read:
		out, e = a.s.Feeds(ctx, p)
	case path == "search" && read:
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		out, e = a.s.Search(ctx, p, r.URL.Query().Get("q"), r.URL.Query().Get("literal") == "true", limit, offset)
	case path == "documents" && read:
		out, e = a.s.Documents(ctx, p)
	case path == "documents" && r.Method == "POST":
		var v domain.Document
		if !body(&v) {
			return
		}
		out, e = a.s.EditDocument(ctx, p, v, 0)
		status = 201
	case parts[0] == "documents" && len(parts) == 2 && read:
		rev, _ := strconv.Atoi(r.URL.Query().Get("revision"))
		out, e = a.s.Document(ctx, p, id, rev)
	case parts[0] == "documents" && len(parts) == 2 && r.Method == "PATCH":
		var v domain.Document
		if !body(&v) {
			return
		}
		v.ID = id
		out, e = a.s.EditDocument(ctx, p, v, revision)
	case parts[0] == "documents" && len(parts) == 3 && parts[2] == "approve" && r.Method == "POST":
		out, e = a.s.ApprovePlan(ctx, p, id, revision)
	case path == "relations" && read:
		out, e = a.s.Relations(ctx, p)
	case path == "relations" && r.Method == "POST":
		var v domain.Relation
		if !body(&v) {
			return
		}
		out, e = a.s.SaveRelation(ctx, p, v)
	case parts[0] == "relations" && len(parts) == 2 && r.Method == "DELETE":
		e = a.s.DeleteRelation(ctx, p, id)
		out = map[string]bool{"ok": e == nil}
	case path == "events" && read:
		out, e = a.s.Events(ctx, p)
	case path == "tokens" && read:
		out, e = a.s.Tokens(ctx, p)
	case path == "tokens" && r.Method == "POST":
		var v domain.Principal
		if !body(&v) {
			return
		}
		var secret string
		var meta domain.Token
		secret, meta, e = a.s.IssueToken(ctx, p, v)
		out = map[string]any{"secret": secret, "token": meta}
		status = 201
	case parts[0] == "tokens" && len(parts) == 2 && r.Method == "DELETE":
		e = a.s.RevokeToken(ctx, p, id)
		out = map[string]bool{"ok": e == nil}
	case path == "export" && read:
		var nodes []domain.Node
		nodes, e = a.s.Nodes(ctx, p)
		if e != nil {
			break
		}
		docs, err := a.s.Documents(ctx, p)
		if err != nil {
			e = err
			break
		}
		relations, err := a.s.Relations(ctx, p)
		if err != nil {
			e = err
			break
		}
		events, err := a.s.Events(ctx, p)
		e = err
		out = map[string]any{"schema_version": 1, "nodes": nodes, "documents": docs, "relations": relations, "events": events}
	default:
		e = app.ErrNotFound
	}
	if e != nil {
		failure(w, e)
		return
	}
	switch v := out.(type) {
	case domain.Node:
		w.Header().Set("ETag", strconv.Quote(strconv.Itoa(v.Revision)))
	case domain.Document:
		w.Header().Set("ETag", strconv.Quote(strconv.Itoa(v.Revision)))
	}
	write(w, status, out)
}
