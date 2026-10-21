package app

import (
	"ainfra/internal/domain"
	"ainfra/internal/domain/version"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

func Assess(p domain.Package, a domain.Advisory) (string, string) {
	if a.Status == "notice_only" {
		return "unknown", "unknown"
	}
	official := a.Distro == "debian" && p.Origin == "Debian" || a.Distro == "ubuntu" && p.Origin == "Ubuntu"
	if !official || p.OriginEvidence == "" || strings.Contains(p.Suite, "backports") || strings.Contains(p.Version, "~bpo") || p.SourceVersion == "" {
		return "unknown", "unknown"
	}
	if a.Withdrawn || a.Status == "not_affected" || a.Fixed == "0" {
		return "not_affected", "unknown"
	}
	if a.Introduced != "" && a.Introduced != "0" {
		c, e := version.Compare(p.SourceVersion, a.Introduced)
		if e != nil {
			return "unknown", "unknown"
		}
		if c < 0 {
			return "not_affected", "unknown"
		}
	}
	if a.Fixed == "" {
		if a.Distro == "ubuntu" {
			return "affected_or_under_investigation", "unknown"
		}
		if a.Status == "open" {
			return "affected", "no_fix_published"
		}
		return "under_investigation", "unknown"
	}
	c, e := version.Compare(p.SourceVersion, a.Fixed)
	if e != nil {
		return "unknown", "unknown"
	}
	if c >= 0 {
		return "not_affected", "unknown"
	}
	rem := "fix_published_not_candidate"
	if p.CandidateSource == a.Source && p.CandidateSourceVersion != "" {
		c, e = version.Compare(p.CandidateSourceVersion, a.Fixed)
		if e == nil && c >= 0 {
			rem = "candidate_fixes"
		}
	}
	return "affected", rem
}

// AdvisorySnapshot is a validated staging source. Walk must be repeatable and
// report read failures, so publication can roll back atomically.
type AdvisorySnapshot interface {
	Walk(func(string, []domain.Advisory) error) error
}
type sliceSnapshot []domain.Advisory

func (records sliceSnapshot) Walk(visit func(string, []domain.Advisory) error) error {
	groups := map[string][]domain.Advisory{}
	for _, a := range records {
		groups[a.Source] = append(groups[a.Source], a)
	}
	for source, ads := range groups {
		if e := visit(source, ads); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) SyncFeed(ctx context.Context, id string, advisories []domain.Advisory, etag, modified string, fetchErr error) error {
	return s.SyncSnapshot(ctx, id, sliceSnapshot(advisories), etag, modified, fetchErr)
}
func (s *Service) SyncSnapshot(ctx context.Context, id string, snapshot AdvisorySnapshot, etag, modified string, fetchErr error) error {
	if fetchErr != nil {
		return s.feedError(ctx, id, fetchErr)
	}
	err := s.store.Write(ctx, func(tx Tx) error {
		f := domain.Feed{ID: id}
		if e := tx.Get("feed", id, &f); e != nil && e != ErrNotFound {
			return e
		}
		f.LastAttempt = s.now()
		f.LastSuccess = s.now()
		f.Error = ""
		f.ETag = etag
		f.SourceModified = modified
		f.Count = 0
		var previous []string
		if e := tx.Get("feed-sources", id, &previous); e != nil && e != ErrNotFound {
			return e
		}
		current := map[string]bool{}
		keys := []string{}
		if e := snapshot.Walk(func(source string, records []domain.Advisory) error {
			if current[source] || source == "" {
				return ErrInvalid
			}
			current[source] = true
			keys = append(keys, source)
			f.Count += len(records)
			return tx.Put("advisory:"+id, source, records)
		}); e != nil {
			return e
		}
		if f.Count == 0 {
			return errors.New("empty_snapshot")
		}
		for _, source := range previous {
			if !current[source] {
				if e := tx.Delete("advisory:"+id, source); e != nil {
					return e
				}
			}
		}
		if e := tx.Put("feed-sources", id, keys); e != nil {
			return e
		}
		if e := tx.Delete("advisories", id); e != nil {
			return e
		}
		if e := tx.Put("feed", id, f); e != nil {
			return e
		}
		nodes, e := decodeList[domain.Node](tx, "node")
		if e != nil {
			return e
		}
		for _, n := range nodes {
			if e = s.reassess(tx, n); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return s.feedError(ctx, id, err)
	}
	return nil
}
func (s *Service) feedError(ctx context.Context, id string, cause error) error {
	err := s.store.Write(ctx, func(tx Tx) error {
		f := domain.Feed{ID: id}
		if e := tx.Get("feed", id, &f); e != nil && e != ErrNotFound {
			return e
		}
		f.LastAttempt = s.now()
		f.Error = "sync_failed"
		return tx.Put("feed", id, f)
	})
	if err != nil {
		return err
	}
	return cause
}

func (s *Service) FeedUnchanged(ctx context.Context, id string) error {
	return s.store.Write(ctx, func(tx Tx) error {
		var f domain.Feed
		if e := tx.Get("feed", id, &f); e != nil {
			return e
		}
		f.LastAttempt = s.now()
		f.LastSuccess = s.now()
		f.Error = ""
		return tx.Put("feed", id, f)
	})
}
func (s *Service) Feeds(ctx context.Context, p domain.Principal) (out []domain.Feed, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Feed](tx, "feed"); return e })
	return
}
func (s *Service) Findings(ctx context.Context, p domain.Principal, node string) (out []domain.Finding, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	out = []domain.Finding{}
	err = s.store.Read(ctx, func(tx Tx) error {
		var fs []domain.Finding
		var e error
		if node == "" {
			fs, e = decodeList[domain.Finding](tx, "finding")
		} else {
			fs, e = decodeNodeList[domain.Finding](tx, "finding", node)
		}
		if e != nil {
			return e
		}
		for _, f := range fs {
			if node == "" || f.NodeID == node {
				if f.Workflow == "suppressed" && !f.SuppressedUntil.After(s.now()) {
					f.Workflow = "open"
				}
				out = append(out, f)
			}
		}
		return nil
	})
	return
}
func (s *Service) reassess(tx Tx, n domain.Node) error {
	var os domain.OSData
	var data domain.PackageData
	sec, ok := n.Observations["packages"]
	if !ok {
		return nil
	}
	if json.Unmarshal(n.Observations["os"].Data, &os) != nil || json.Unmarshal(sec.Data, &data) != nil {
		return nil
	}
	feeds, e := decodeList[domain.Feed](tx, "feed")
	if e != nil {
		return e
	}
	seen := map[string]bool{}
	sources := map[string]bool{}
	packagesBySource := map[string][]domain.Package{}
	for _, p := range data.Packages {
		sources[p.Source] = true
		packagesBySource[p.Source] = append(packagesBySource[p.Source], p)
	}
	for _, feed := range feeds {
		if feed.ID != os.ID {
			continue
		}
		ads := []domain.Advisory{}
		for source := range packagesBySource {
			var records []domain.Advisory
			if e := tx.Get("advisory:"+feed.ID, source, &records); e != nil && e != ErrNotFound {
				return e
			}
			ads = append(ads, records...)
		}
		for _, a := range ads {
			if a.Distro != os.ID || !(a.Release == os.Codename || a.Release == os.Version || strings.TrimSuffix(a.Release, ":LTS") == os.Version) {
				continue
			}
			key := hash([]string{n.ID, a.Source, a.ID, a.Channel})
			group := packagesBySource[a.Source]
			if len(group) == 0 {
				continue
			}
			seen[key] = true
			f := domain.Finding{ID: key, NodeID: n.ID, FirstSeen: s.now(), Workflow: "open"}
			oldErr := tx.Get("finding", key, &f)
			if oldErr != nil && oldErr != ErrNotFound {
				return oldErr
			}
			old := f
			f.Advisory = a
			f.Packages = []string{}
			f.Assessment = "not_affected"
			f.Remediation = "unknown"
			for _, p := range group {
				as, rem := Assess(p, a)
				if as != "not_affected" {
					f.Packages = append(f.Packages, p.Name+":"+p.Arch)
					if f.Assessment == "not_affected" || as == "affected" {
						f.Assessment = as
						f.Remediation = rem
					}
				}
			}
			if f.Assessment == "not_affected" && oldErr == ErrNotFound {
				continue
			}
			f.InventoryAt = sec.ObservedAt
			f.FeedAt = feed.LastSuccess
			f.LastEvaluated = s.now()
			sort.Strings(f.Packages)
			if f.Assessment == "not_affected" {
				f.Workflow = "resolved"
			} else if f.Workflow == "resolved" {
				f.Workflow = "open"
			}
			setClosureTime(&f, old, s.now())
			if e = tx.Put("finding", key, f); e != nil {
				return e
			}
			if oldErr != nil || old.Assessment != f.Assessment || old.Remediation != f.Remediation || old.Workflow != f.Workflow {
				if e = s.event(tx, domain.Principal{ID: "evaluator"}, "finding.changed", key, f.Assessment+"/"+f.Remediation); e != nil {
					return e
				}
			}
		}
	}
	old, e := decodeNodeList[domain.Finding](tx, "finding", n.ID)
	if e != nil {
		return e
	}
	for _, f := range old {
		if f.NodeID != n.ID || seen[f.ID] {
			continue
		}
		previous := f
		if !sources[f.Advisory.Source] {
			f.Workflow = "resolved"
			f.Reason = "source absent from complete inventory"
		} else {
			f.Assessment = "unknown"
			f.Remediation = "unknown"
			f.Reason = "advisory coverage lost"
			if f.Workflow == "resolved" {
				f.Workflow = "open"
			}
		}
		f.InventoryAt = sec.ObservedAt
		f.LastEvaluated = s.now()
		setClosureTime(&f, previous, s.now())
		if e = tx.Put("finding", f.ID, f); e != nil {
			return e
		}
	}
	return refreshFindingSummary(tx, n.ID)
}
func (s *Service) Suppress(ctx context.Context, p domain.Principal, id, reason string, until time.Time) error {
	if p.Role != "admin" {
		return ErrForbidden
	}
	if strings.TrimSpace(reason) == "" || !until.After(s.now()) || until.After(s.now().AddDate(1, 0, 0)) {
		return ErrInvalid
	}
	return s.store.Write(ctx, func(tx Tx) error {
		var f domain.Finding
		if e := tx.Get("finding", id, &f); e != nil {
			return e
		}
		f.Workflow = "suppressed"
		f.Reason = reason
		f.SuppressedUntil = until
		if e := tx.Put("finding", id, f); e != nil {
			return e
		}
		if e := refreshFindingSummary(tx, f.NodeID); e != nil {
			return e
		}
		return s.event(tx, p, "finding.suppressed", id, reason)
	})
}

// A heartbeat must not extend the retention period of a closed finding.
func setClosureTime(f *domain.Finding, previous domain.Finding, now time.Time) {
	if f.Workflow != "resolved" {
		f.ResolvedAt = time.Time{}
		return
	}
	if previous.Workflow != "resolved" {
		f.ResolvedAt = now
		return
	}
	if f.ResolvedAt.IsZero() {
		f.ResolvedAt = previous.LastEvaluated // Legacy records have no closure timestamp.
		if f.ResolvedAt.IsZero() {
			f.ResolvedAt = now
		}
	}
}
