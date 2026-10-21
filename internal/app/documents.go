package app

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) EditDocument(ctx context.Context, p domain.Principal, d domain.Document, revision int) (out domain.Document, err error) {
	if p.Role != "editor" && p.Role != "admin" {
		return out, ErrForbidden
	}
	if d.Title == "" || len(d.Title) > 300 || len(d.Body) > 1024*1024 || !contains([]string{"note", "access", "runbook", "maintenance_plan"}, d.Kind) {
		return out, ErrInvalid
	}
	err = s.store.Write(ctx, func(tx Tx) error {
		for _, id := range d.Nodes {
			var n domain.Node
			if e := tx.Get("node", id, &n); e != nil {
				return e
			}
		}
		if d.ID == "" {
			d.ID = s.id()
			d.Revision = 1
		} else {
			var old domain.Document
			if e := tx.Get("document", d.ID, &old); e != nil {
				return e
			}
			if old.Revision != revision {
				return ErrConflict
			}
			d.Revision = old.Revision + 1
		}
		d.Author = p.ID
		d.UpdatedAt = s.now()
		d.Status = "draft"
		d.ApprovedRevision = 0
		d.ApprovedHash = ""
		d.Approver = ""
		d.NeedsRevalidation = false
		d.EvidenceHash = ""
		if d.Kind == "maintenance_plan" {
			d.EvidenceHash = evidence(tx, d.Nodes)
		}
		if e := tx.Put("document", d.ID, d); e != nil {
			return e
		}
		if e := tx.Put("revision", fmt.Sprintf("%s:%09d", d.ID, d.Revision), d); e != nil {
			return e
		}
		out = d
		return s.event(tx, p, "document.edited", d.ID, fmt.Sprint(d.Revision))
	})
	return
}
func evidence(tx Tx, nodes []string) string {
	v := map[string]any{}
	for _, id := range nodes {
		var n domain.Node
		if tx.Get("node", id, &n) == nil {
			v[id] = n.Observations["packages"].Data
		}
	}
	docs, _ := decodeList[domain.Document](tx, "document")
	for _, d := range docs {
		if d.Kind == "maintenance_plan" {
			continue
		}
		for _, id := range nodes {
			if contains(d.Nodes, id) {
				v["doc:"+d.ID] = d.Revision
			}
		}
	}
	return hash(v)
}
func (s *Service) Document(ctx context.Context, p domain.Principal, id string, revision int) (d domain.Document, err error) {
	if !reader(p) {
		return d, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error {
		kind := "document"
		key := id
		if revision > 0 {
			kind = "revision"
			key = fmt.Sprintf("%s:%09d", id, revision)
		}
		if e := tx.Get(kind, key, &d); e != nil {
			return e
		}
		d.NeedsRevalidation = d.Kind == "maintenance_plan" && d.EvidenceHash != evidence(tx, d.Nodes)
		return nil
	})
	return
}
func (s *Service) ApprovePlan(ctx context.Context, p domain.Principal, id string, rev int) (d domain.Document, err error) {
	if p.Role != "admin" {
		return d, ErrForbidden
	}
	err = s.store.Write(ctx, func(tx Tx) error {
		if e := tx.Get("document", id, &d); e != nil {
			return e
		}
		if d.Kind != "maintenance_plan" || d.Author == p.ID {
			return ErrForbidden
		}
		if d.Revision != rev || d.EvidenceHash != evidence(tx, d.Nodes) {
			return ErrConflict
		}
		d.Status = "approved"
		d.Approver = p.ID
		d.ApprovedRevision = d.Revision
		d.ApprovedHash = hash(d.Body)
		if e := tx.Put("document", id, d); e != nil {
			return e
		}
		return s.event(tx, p, "plan.approved", id, d.ApprovedHash)
	})
	return
}
func (s *Service) Documents(ctx context.Context, p domain.Principal) (out []domain.Document, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Document](tx, "document"); return e })
	return
}
func (s *Service) Search(ctx context.Context, p domain.Principal, q string, literal bool, limit, offset int) ([]json.RawMessage, error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	if len(q) > 300 || limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalid
	}
	return s.store.Search(ctx, q, literal, limit, offset)
}
func (s *Service) Events(ctx context.Context, p domain.Principal) (out []domain.Event, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Event](tx, "event"); return e })
	return
}

func (s *Service) SearchCount(ctx context.Context, p domain.Principal, q string, literal bool) (int, error) {
	if !reader(p) {
		return 0, ErrForbidden
	}
	if len(q) > 300 {
		return 0, ErrInvalid
	}
	return s.store.CountSearch(ctx, q, literal)
}
