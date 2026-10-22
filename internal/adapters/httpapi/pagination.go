package httpapi

import (
	"ainfra/internal/app"
	"ainfra/internal/domain"
	"encoding/json"
	"net/http"
	"strconv"
)

type pagination struct{ limit, offset int }

func parsePagination(r *http.Request) (p pagination, e error) {
	p.limit = 100
	for key, dest := range map[string]*int{"limit": &p.limit, "offset": &p.offset} {
		if raw := r.URL.Query().Get(key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return p, app.ErrInvalid
			}
			*dest = n
		}
	}
	if p.limit < 1 || p.limit > 1000 || p.offset < 0 || p.offset > 10000000 {
		return p, app.ErrInvalid
	}
	return p, nil
}
func page[T any](w http.ResponseWriter, p pagination, rows []T) []T {
	total := len(rows)
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	w.Header().Set("X-Page-Limit", strconv.Itoa(p.limit))
	w.Header().Set("X-Page-Offset", strconv.Itoa(p.offset))
	if p.offset >= total {
		return []T{}
	}
	end := p.offset + p.limit
	if end > total {
		end = total
	}
	bytes := 0
	for i := p.offset; i < end; i++ {
		b, _ := json.Marshal(rows[i])
		if bytes+len(b) > 16<<20 && i > p.offset {
			end = i
			break
		}
		bytes += len(b) + 1
	}
	if end < total {
		w.Header().Set("X-Next-Offset", strconv.Itoa(end))
	}
	return rows[p.offset:end]
}
func paginate(w http.ResponseWriter, p pagination, value any) any {
	switch rows := value.(type) {
	case []domain.Node:
		return page(w, p, rows)
	case []domain.NodeBrief:
		return page(w, p, rows)
	case []domain.Document:
		return page(w, p, rows)
	case []domain.JournalEntry:
		return page(w, p, rows)
	case []domain.RelationSuggestion:
		return page(w, p, rows)
	case []domain.Relation:
		return page(w, p, rows)
	case []domain.Token:
		return page(w, p, rows)
	case []domain.Attention:
		return page(w, p, rows)
	case []domain.Finding:
		return page(w, p, rows)
	case []domain.Feed:
		return page(w, p, rows)
	case []domain.Event:
		return page(w, p, rows)
	case domain.PackageData:
		rows.Packages = page(w, p, rows.Packages)
		return rows
	default:
		return value
	}
}

func filterByNode(value any, node string) any {
	if node == "" {
		return value
	}
	switch rows := value.(type) {
	case []domain.Document:
		out := []domain.Document{}
		for _, r := range rows {
			for _, id := range r.Nodes {
				if id == node {
					out = append(out, r)
					break
				}
			}
		}
		return out
	case []domain.Relation:
		out := []domain.Relation{}
		for _, r := range rows {
			if r.Source == node || r.Target == node {
				out = append(out, r)
			}
		}
		return out
	case []domain.Event:
		out := []domain.Event{}
		for _, r := range rows {
			if r.Entity == node {
				out = append(out, r)
			}
		}
		return out
	default:
		return value
	}
}
