package app

import (
	"ainfra/internal/domain"
	"ainfra/internal/domain/version"
	"sort"
)

func assessRanges(p domain.Package, a domain.Advisory) (string, string) {
	affected, fix, err := includedInRanges(p.SourceVersion, a.Ranges)
	if err != nil {
		return "unknown", "unknown"
	}
	if !affected {
		return "not_affected", "unknown"
	}
	if !fix {
		return "affected_or_under_investigation", "unknown"
	}
	if candidateTrusted(p, a) && p.CandidateSource == a.Source && p.CandidateSourceVersion != "" {
		candidate, _, err := includedInRanges(p.CandidateSourceVersion, a.Ranges)
		comparison, compareErr := version.Compare(p.CandidateSourceVersion, p.SourceVersion)
		if err == nil && compareErr == nil && !candidate && comparison > 0 {
			return "affected", "candidate_fixes"
		}
	}
	return "affected", "fix_published_not_candidate"
}

func includedInRanges(v string, ranges []domain.AffectedRange) (bool, bool, error) {
	anyAffected, anyFix := false, true
	for _, r := range ranges {
		events := append([]domain.VersionEvent(nil), r.Events...)
		beforeLimit, hasLimit := false, false
		for _, event := range events {
			for _, value := range []string{event.Introduced, event.Fixed, event.LastAffected, event.Limit} {
				if value != "" && value != "*" {
					if _, err := version.Compare(value, value); err != nil {
						return false, false, err
					}
				}
			}
			if event.Limit != "" {
				hasLimit = true
				if event.Limit == "*" {
					beforeLimit = true
					continue
				}
				c, err := version.Compare(v, event.Limit)
				if err != nil {
					return false, false, err
				}
				beforeLimit = beforeLimit || c < 0
			}
		}
		if hasLimit && !beforeLimit {
			continue
		}
		value := func(e domain.VersionEvent) string {
			if e.Introduced != "" {
				return e.Introduced
			}
			if e.Fixed != "" {
				return e.Fixed
			}
			if e.LastAffected != "" {
				return e.LastAffected
			}
			return "0"
		}
		sort.SliceStable(events, func(i, j int) bool { c, _ := version.Compare(value(events[i]), value(events[j])); return c < 0 })
		affected, fix := false, false
		for _, event := range events {
			if event.Limit != "" {
				continue
			}
			c, err := version.Compare(v, value(event))
			if err != nil {
				return false, false, err
			}
			switch {
			case event.Introduced != "":
				if event.Introduced == "0" || c >= 0 {
					affected = true
				}
			case event.Fixed != "":
				if c >= 0 {
					affected = false
				} else {
					fix = true
				}
			case event.LastAffected != "":
				if c > 0 {
					affected = false
				}
			}
		}
		if affected {
			anyAffected = true
			anyFix = anyFix && fix
		}
	}
	return anyAffected, anyFix, nil
}
