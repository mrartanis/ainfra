// Package ui is an independent HTTP presentation component. It only accesses the API.
package ui

import (
	"ainfra/internal/apiclient"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*
var assets embed.FS

type session struct {
	ID, CSRF, Token, Role string
	Expires               time.Time
}
type rate struct {
	Count int
	Until time.Time
}
type UI struct {
	base      string
	client    *http.Client
	dev       bool
	now       func() time.Time
	mu        sync.Mutex
	sessions  map[string]session
	rates     map[string]rate
	templates *template.Template
}

func New(base string, client *http.Client, dev bool, now func() time.Time) *UI {
	funcs := template.FuncMap{"json": func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }, "join": func(v any) string {
		a, ok := v.([]any)
		if !ok {
			return ""
		}
		ss := []string{}
		for _, x := range a {
			ss = append(ss, fmt.Sprint(x))
		}
		return strings.Join(ss, ", ")
	}, "short": func(v any) string {
		s := fmt.Sprint(v)
		if strings.HasPrefix(s, "0001-") {
			return "неизвестно"
		}
		return s
	}, "count": func(v any) int { a, _ := v.([]any); return len(a) }}
	return &UI{base: base, client: client, dev: dev, now: now, sessions: map[string]session{}, rates: map[string]rate{}, templates: template.Must(template.New("ui").Funcs(funcs).ParseFS(assets, "templates/*.html"))}
}
func random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (u *UI) session(w http.ResponseWriter, r *http.Request) session {
	u.mu.Lock()
	defer u.mu.Unlock()
	for id, s := range u.sessions {
		if !s.Expires.After(u.now()) {
			delete(u.sessions, id)
		}
	}
	if c, e := r.Cookie("infra_session"); e == nil {
		if s, ok := u.sessions[c.Value]; ok {
			return s
		}
	}
	if len(u.sessions) >= 1024 {
		return session{}
	}
	s := session{ID: random(), CSRF: random(), Expires: u.now().Add(8 * time.Hour)}
	u.sessions[s.ID] = s
	http.SetCookie(w, &http.Cookie{Name: "infra_session", Value: s.ID, Path: "/", Secure: !u.dev, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	return s
}
func (u *UI) render(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if e := u.templates.ExecuteTemplate(w, "page", data); e != nil {
		return
	}
}
func (u *UI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/style.css" {
		w.Header().Set("Content-Type", "text/css")
		b, _ := assets.ReadFile("templates/style.css")
		w.Write(b)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	s := u.session(w, r)
	if s.ID == "" {
		http.Error(w, "Слишком много сессий", 429)
		return
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if e := r.ParseForm(); e != nil {
			http.Error(w, "Слишком большой запрос", 413)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")), []byte(s.CSRF)) != 1 {
			http.Error(w, "CSRF: обновите страницу", 403)
			return
		}
	}
	data := map[string]any{"CSRF": s.CSRF, "Role": s.Role, "Page": "nodes", "Title": "Узлы"}
	client := &apiclient.Client{Base: u.base, Token: s.Token, HTTP: u.client}
	ctx := r.Context()
	get := func(path, key string) error {
		var v any
		if e := client.Do(ctx, "GET", path, nil, &v, 0); e != nil {
			return e
		}
		data[key] = v
		return nil
	}
	fail := func(e error) {
		data["Page"] = "error"
		data["Title"] = "Не удалось выполнить запрос"
		data["Error"] = e.Error()
		u.render(w, data)
	}
	if r.URL.Path == "/login" {
		data["Page"] = "login"
		data["Title"] = "Вход"
		if r.Method == "POST" {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			u.mu.Lock()
			for k, v := range u.rates {
				if !v.Until.After(u.now()) {
					delete(u.rates, k)
				}
			}
			v := u.rates[ip]
			if v.Count >= 5 || len(u.rates) > 4096 {
				u.mu.Unlock()
				http.Error(w, "Повторите через минуту", 429)
				return
			}
			v.Count++
			v.Until = u.now().Add(time.Minute)
			u.rates[ip] = v
			u.mu.Unlock()
			token := strings.TrimSpace(r.Form.Get("token"))
			client.Token = token
			var me map[string]any
			if e := client.Do(ctx, "GET", "me", nil, &me, 0); e != nil {
				data["Error"] = "Неверный токен или API недоступен"
				u.render(w, data)
				return
			}
			role, _ := me["role"].(string)
			if role != "admin" && role != "editor" && role != "reader" {
				http.Error(w, "Недостаточно прав", 403)
				return
			}
			u.mu.Lock()
			delete(u.sessions, s.ID)
			s.ID = random()
			s.CSRF = random()
			s.Token = token
			s.Role = role
			u.sessions[s.ID] = s
			u.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "infra_session", Value: s.ID, Path: "/", Secure: !u.dev, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
			http.Redirect(w, r, "/", 303)
			return
		}
		u.render(w, data)
		return
	}
	if s.Token == "" {
		http.Redirect(w, r, "/login", 303)
		return
	}
	if r.URL.Path == "/logout" && r.Method == "POST" {
		u.mu.Lock()
		delete(u.sessions, s.ID)
		u.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "infra_session", Path: "/", MaxAge: -1, Secure: !u.dev, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/login", 303)
		return
	}
	if r.Method == "POST" {
		dest, e := u.mutate(ctx, client, r, data)
		if e != nil {
			fail(e)
			return
		}
		if dest == "" {
			u.render(w, data)
		} else {
			http.Redirect(w, r, dest, 303)
		}
		return
	}
	path := r.URL.Path
	switch {
	case path == "/":
		if e := get("attention", "Attention"); e != nil {
			fail(e)
			return
		}
		if e := get("nodes", "Nodes"); e != nil {
			fail(e)
			return
		}
		data["Filter"] = r.URL.Query().Get("filter")
		data["Sort"] = r.URL.Query().Get("sort")
	case path == "/nodes/new":
		data["Page"] = "node-edit"
		data["Title"] = "Добавить узел"
		data["Node"] = map[string]any{}
	case strings.HasPrefix(path, "/nodes/"):
		id := strings.TrimPrefix(path, "/nodes/")
		if e := get("nodes/"+url.PathEscape(id), "Node"); e != nil {
			fail(e)
			return
		}
		data["Page"] = "node"
		data["Title"] = "Карточка узла"
		tab := r.URL.Query().Get("tab")
		data["Tab"] = tab
		if r.URL.Query().Get("edit") == "true" {
			data["Page"] = "node-edit"
		} else {
			for _, x := range [][2]string{{"nodes/" + id + "/findings", "Findings"}, {"nodes/" + id + "/packages", "Packages"}, {"documents", "Documents"}, {"relations", "Relations"}, {"nodes", "Nodes"}, {"events", "Events"}} {
				if e := get(x[0], x[1]); e != nil {
					fail(e)
					return
				}
			}
		}
	case strings.HasPrefix(path, "/context/"):
		var text string
		if e := client.Do(ctx, "GET", "nodes/"+url.PathEscape(strings.TrimPrefix(path, "/context/"))+"/context", nil, &text, 0); e != nil {
			fail(e)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="context.md"`)
		w.Write([]byte(text))
		return
	case path == "/docs/new":
		data["Page"] = "document"
		data["Title"] = "Новая запись"
		data["Document"] = map[string]any{"kind": "note", "nodes": []string{r.URL.Query().Get("node")}}
	case strings.HasPrefix(path, "/docs/"):
		if e := get("documents/"+url.PathEscape(strings.TrimPrefix(path, "/docs/"))+"?revision="+url.QueryEscape(r.URL.Query().Get("revision")), "Document"); e != nil {
			fail(e)
			return
		}
		data["Page"] = "document"
		data["Title"] = "Запись"
	case path == "/search":
		data["Page"] = "search"
		data["Title"] = "Поиск"
		data["Query"] = r.URL.Query().Get("q")
		if e := get("search?q="+url.QueryEscape(r.URL.Query().Get("q"))+"&literal="+url.QueryEscape(r.URL.Query().Get("literal")), "Results"); e != nil {
			fail(e)
			return
		}
	case path == "/settings":
		data["Page"] = "settings"
		data["Title"] = "Настройки"
		for _, x := range [][2]string{{"feed-status", "Feeds"}, {"settings", "Settings"}, {"nodes", "Nodes"}} {
			if e := get(x[0], x[1]); e != nil {
				fail(e)
				return
			}
		}
		if s.Role == "admin" {
			if e := get("tokens", "Tokens"); e != nil {
				fail(e)
				return
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	u.render(w, data)
}
func (u *UI) mutate(ctx context.Context, c *apiclient.Client, r *http.Request, data map[string]any) (string, error) {
	f := r.Form
	rev, _ := strconv.Atoi(f.Get("revision"))
	split := func(s string) []string {
		out := []string{}
		for _, x := range strings.Split(s, ",") {
			if v := strings.TrimSpace(x); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	switch r.URL.Path {
	case "/nodes/save":
		n := map[string]any{}
		id := f.Get("id")
		method, path := "POST", "nodes"
		if id != "" {
			if e := c.Do(ctx, "GET", "nodes/"+url.PathEscape(id), nil, &n, 0); e != nil {
				return "", e
			}
			method = "PATCH"
			path += "/" + url.PathEscape(id)
		}
		for _, k := range []string{"name", "purpose", "kind", "criticality", "lifecycle", "hostname"} {
			n[k] = f.Get(k)
		}
		n["addresses"] = split(f.Get("addresses"))
		n["tags"] = split(f.Get("tags"))
		if f.Get("next_due") != "" {
			due, e := time.Parse("2006-01-02", f.Get("next_due"))
			if e != nil {
				return "", e
			}
			interval, _ := strconv.Atoi(f.Get("payment_interval"))
			n["payment"] = map[string]any{"next_due": due, "interval": interval, "unit": f.Get("payment_unit"), "timezone": "UTC", "amount": f.Get("amount"), "currency": f.Get("currency")}
		}
		var out map[string]any
		if e := c.Do(ctx, method, path, n, &out, rev); e != nil {
			return "", e
		}
		return "/nodes/" + fmt.Sprint(out["id"]), nil
	case "/docs/save":
		id := f.Get("id")
		v := map[string]any{"id": id, "title": f.Get("title"), "kind": f.Get("kind"), "body": f.Get("body"), "nodes": split(f.Get("nodes"))}
		method, path := "POST", "documents"
		if id != "" {
			method = "PATCH"
			path += "/" + url.PathEscape(id)
		}
		var out map[string]any
		if e := c.Do(ctx, method, path, v, &out, rev); e != nil {
			return "", e
		}
		return "/docs/" + fmt.Sprint(out["id"]), nil
	case "/docs/approve":
		id := f.Get("id")
		return "/docs/" + id, c.Do(ctx, "POST", "documents/"+url.PathEscape(id)+"/approve", nil, nil, rev)
	case "/relations/save":
		return "/nodes/" + f.Get("source"), c.Do(ctx, "POST", "relations", map[string]any{"source": f.Get("source"), "target": f.Get("target"), "type": f.Get("type"), "name": f.Get("name")}, nil, 0)
	case "/tokens/revoke":
		return "/settings", c.Do(ctx, "DELETE", "tokens/"+url.PathEscape(f.Get("id")), nil, nil, 0)
	case "/tokens/issue":
		var out map[string]any
		if e := c.Do(ctx, "POST", "tokens", map[string]any{"role": f.Get("role"), "nodes": split(f.Get("nodes")), "instance": f.Get("instance")}, &out, 0); e != nil {
			return "", e
		}
		data["Page"] = "token"
		data["Title"] = "Токен создан"
		data["Secret"] = out["secret"]
		return "", nil
	case "/pay":
		id := f.Get("id")
		return "/nodes/" + id, c.Do(ctx, "POST", "nodes/"+url.PathEscape(id)+"/pay", nil, nil, rev)
	case "/services/save", "/backups/save":
		id := f.Get("id")
		n := map[string]any{}
		if e := c.Do(ctx, "GET", "nodes/"+url.PathEscape(id), nil, &n, 0); e != nil {
			return "", e
		}
		if r.URL.Path == "/services/save" {
			rows, _ := n["services"].([]any)
			rows = append(rows, map[string]any{"name": f.Get("name"), "url": f.Get("url"), "purpose": f.Get("purpose"), "exposure": f.Get("exposure"), "instructions": f.Get("instructions")})
			n["services"] = rows
		} else {
			rows, _ := n["backups"].([]any)
			interval, _ := strconv.Atoi(f.Get("interval"))
			rows = append(rows, map[string]any{"id": f.Get("check_id"), "label": f.Get("label"), "destination": f.Get("destination"), "interval_hours": interval, "grace_hours": 2})
			n["backups"] = rows
		}
		return "/nodes/" + id, c.Do(ctx, "PATCH", "nodes/"+url.PathEscape(id), n, nil, rev)
	default:
		return "", fmt.Errorf("неизвестная операция")
	}
}
