package app

import (
	"ainfra/internal/domain"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
func reader(p domain.Principal) bool { return contains([]string{"reader", "editor", "admin"}, p.Role) }
func (s *Service) event(tx Tx, p domain.Principal, kind, id, detail string) error {
	eventID := s.id()
	return tx.Put("event", eventID, domain.Event{ID: eventID, Kind: kind, Entity: id, Actor: p.ID, At: s.now(), Detail: detail})
}
func (s *Service) IngestReport(ctx context.Context, p domain.Principal, r domain.Report) (ack domain.Ack, err error) {
	ack = domain.Ack{ID: r.ID, Status: "accepted"}
	if !contains([]string{"agent", "probe"}, p.Role) || !contains(p.Nodes, r.NodeID) || p.Instance != r.Instance {
		return ack, ErrForbidden
	}
	if r.SchemaVersion != 1 || r.ID == "" || len(r.ID) > 100 || r.Sequence == 0 || r.ObservedAt.IsZero() || r.ObservedAt.After(s.now().Add(5*time.Minute)) || len(r.Collectors) > 16 {
		return ack, ErrInvalid
	}
	totalBytes := 0
	for name, section := range r.Collectors {
		totalBytes += len(section.Data)
		if totalBytes > 16<<20 || len(section.ErrorCode) > 128 {
			return ack, ErrInvalid
		}
		if len(section.Data) > 0 {
			if e := validateObservation(name, section.Data); e != nil {
				return ack, e
			}
		}
		if name == "checks" && len(section.Data) > 0 {
			if e := ValidateChecks(section.Data); e != nil {
				return ack, e
			}
			var checks []Check
			_ = json.Unmarshal(section.Data, &checks)
			for _, check := range checks {
				if check.ObservedAt.After(s.now().Add(5*time.Minute)) || check.ObservedAt.After(r.ObservedAt.Add(5*time.Minute)) || check.ObservedAt.After(section.ObservedAt.Add(5*time.Minute)) {
					return ack, ErrInvalid
				}
			}
		}
		if !contains([]string{"os", "packages", "network", "wireguard", "checks", "disks"}, name) || !contains([]string{"ok", "partial", "error", "unsupported", "disabled"}, section.Status) {
			return ack, ErrInvalid
		}
		if section.Status == "ok" || section.Status == "partial" {
			if section.ObservedAt.IsZero() || section.ObservedAt.After(s.now().Add(5*time.Minute)) || section.ObservedAt.After(r.ObservedAt.Add(5*time.Minute)) || !json.Valid(section.Data) {
				return ack, ErrInvalid
			}
		}
		if name == "packages" && len(section.Data) > 0 {
			var data domain.PackageData
			if json.Unmarshal(section.Data, &data) != nil || len(data.Packages) > 100000 {
				return ack, ErrInvalid
			}
		}
	}
	err = s.store.Write(ctx, func(tx Tx) error {
		var previous struct {
			Hash string `json:"hash"`
		}
		e := tx.Get("receipt", r.ID, &previous)
		if e == nil {
			if previous.Hash != hash(r) {
				return ErrConflict
			}
			ack.Status = "duplicate"
			return nil
		}
		if e != ErrNotFound {
			return e
		}
		var n domain.Node
		if e = tx.Get("node", r.NodeID, &n); e != nil {
			return e
		}
		if n.Instance != "" && n.Instance != r.Instance {
			return ErrConflict
		}
		if e = tx.Put("receipt", r.ID, map[string]any{"hash": hash(r), "node_id": r.NodeID, "received_at": s.now()}); e != nil {
			return e
		}
		if r.Sequence <= n.Sequence {
			ack.Status = "out_of_order"
			return nil
		}
		n.Instance = r.Instance
		n.Sequence = r.Sequence
		n.LastReport = r.ObservedAt
		n.ReceivedAt = s.now()
		if n.Observations == nil {
			n.Observations = map[string]domain.Section{}
		}
		if n.CollectorStatus == nil {
			n.CollectorStatus = map[string]domain.Section{}
		}
		for k, v := range r.Collectors {
			status := v
			status.Data = nil
			n.CollectorStatus[k] = status
			// Each typed check is independently timestamped; a failed sibling
			// must not hide a valid backup failure or success.
			if k == "checks" && (v.Status == "ok" || v.Status == "partial") && len(v.Data) > 0 {
				if e := applyChecks(&n, v.Data, v.Status == "ok" && v.Complete); e != nil {
					return e
				}
			}
			if v.Status == "ok" && v.Complete {
				old := n.Observations[k]
				if old.ObservedAt.IsZero() || !v.ObservedAt.Before(old.ObservedAt) {
					n.Observations[k] = v
				}
			}
		}
		if e = putNode(tx, n); e != nil {
			return e
		}
		return s.reassess(tx, n)
	})
	return
}
func (s *Service) SaveNode(ctx context.Context, p domain.Principal, n domain.Node, revision int) (out domain.Node, err error) {
	if p.Role != "admin" {
		return out, ErrForbidden
	}
	if strings.TrimSpace(n.Name) == "" || len(n.Name) > 200 {
		return out, ErrInvalid
	}
	if n.Payment != nil {
		if _, e := NextDue(*n.Payment); e != nil {
			return out, e
		}
	}
	if len(n.WireGuardPublicKeys) > 32 {
		return out, ErrInvalid
	}
	seenKeys := map[string]bool{}
	for _, key := range n.WireGuardPublicKeys {
		raw, e := base64.StdEncoding.DecodeString(key)
		if e != nil || len(raw) != 32 || base64.StdEncoding.EncodeToString(raw) != key || seenKeys[key] {
			return out, ErrInvalid
		}
		seenKeys[key] = true
	}
	err = s.store.Write(ctx, func(tx Tx) error {
		if n.ID == "" {
			n.ID = s.id()
			n.Slug = n.ID
			n.Revision = 1
			n.Sequence = 0
			n.PaymentChecks = nil
			n.Instance = ""
			n.LastReport = time.Time{}
			n.ReceivedAt = time.Time{}
			n.Observations = map[string]domain.Section{}
			n.CollectorStatus = map[string]domain.Section{}
		} else {
			var old domain.Node
			if e := tx.Get("node", n.ID, &old); e != nil {
				return e
			}
			if revision != old.Revision {
				return ErrConflict
			}
			n.Revision = old.Revision + 1
			n.Slug = old.Slug
			// A manual form may predate a collector update: report ingestion does
			// not change the manual revision. Retain newer observed backup facts.
			previousBackups := make(map[string]domain.Backup, len(old.Backups))
			for _, b := range old.Backups {
				previousBackups[b.ID] = b
			}
			for i := range n.Backups {
				b := &n.Backups[i]
				if observed, ok := previousBackups[b.ID]; ok && observed.ObservedAt.After(b.ObservedAt) {
					b.ObservedAt = observed.ObservedAt
					b.LastAttempt, b.LastAttemptStatus = observed.LastAttempt, observed.LastAttemptStatus
					b.LastSuccess, b.LastRestoreTest = observed.LastSuccess, observed.LastRestoreTest
					b.Evidence = observed.Evidence
				}
			}
			n.Observations = old.Observations
			n.CollectorStatus = old.CollectorStatus
			n.LastReport = old.LastReport
			n.ReceivedAt = old.ReceivedAt
			n.Sequence = old.Sequence
			n.Instance = old.Instance
			n.PaymentChecks = old.PaymentChecks
		}
		if n.Kind == "" {
			n.Kind = "other"
		}
		if n.Lifecycle == "" {
			n.Lifecycle = "active"
		}
		if n.Criticality == "" {
			n.Criticality = "normal"
		}
		if !contains([]string{"physical", "vm", "router", "other"}, n.Kind) || !contains([]string{"active", "retired"}, n.Lifecycle) || !contains([]string{"low", "normal", "high"}, n.Criticality) {
			return ErrInvalid
		}
		if e := putNode(tx, n); e != nil {
			return e
		}
		out = n
		return s.event(tx, p, "node.saved", n.ID, fmt.Sprint(n.Revision))
	})
	return
}
func (s *Service) Node(ctx context.Context, p domain.Principal, id string) (n domain.Node, err error) {
	if !reader(p) {
		return n, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { return tx.Get("node", id, &n) })
	return
}
func (s *Service) Nodes(ctx context.Context, p domain.Principal) (out []domain.Node, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Node](tx, "node-summary"); return e })
	return
}
