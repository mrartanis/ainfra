package domain

import (
	"ainfra/internal/domain/version"
	"encoding/json"
	"sort"
	"time"
)

// SummarizeNode retains timestamps and coverage while removing per-package and
// runtime detail. It is materialized on writes, never computed by list queries.
func SummarizeNode(n Node) Node {
	observations := map[string]Section{}
	if os, ok := n.Observations["os"]; ok {
		observations["os"] = os
	}
	if packages, ok := n.Observations["packages"]; ok {
		var data PackageData
		if json.Unmarshal(packages.Data, &data) == nil {
			data = CountPackages(data)
			data.Packages = nil
			packages.Data, _ = json.Marshal(data)
			observations["packages"] = packages
		}
	}
	if n.PaymentChecks != nil {
		checks := make(map[string]PaymentObservation, len(n.PaymentChecks))
		for id, c := range n.PaymentChecks {
			c.Evidence = ""
			checks[id] = c
		}
		n.PaymentChecks = checks
	}
	n.Observations = observations
	return n
}

// CountPackages derives totals from the complete inventory, never from a page.
// A nil package list denotes a summary or missing inventory: preserve its
// computed counters (or null), rather than inventing zeroes.
func CountPackages(data PackageData) PackageData {
	if data.Packages == nil {
		return data
	}
	count, unknown, updates := len(data.Packages), 0, 0
	for _, p := range data.Packages {
		if p.Origin == "" || p.SourceVersion == "" {
			unknown++
		}
		if c, e := version.Compare(p.Candidate, p.Version); e == nil && c > 0 {
			updates++
		}
	}
	data.PackageCount, data.UnknownOrigins, data.Updates = &count, &unknown, &updates
	return data
}

// NormalizeNodeCounters also repairs historical reports whose counters were
// serialized as zero before the server computed them.
func NormalizeNodeCounters(n Node) Node {
	observations := make(map[string]Section, len(n.Observations))
	for kind, section := range n.Observations {
		if kind == "packages" {
			var data PackageData
			if json.Unmarshal(section.Data, &data) == nil {
				section.Data, _ = json.Marshal(CountPackages(data))
			}
		}
		observations[kind] = section
	}
	n.Observations = observations
	return n
}

type NodeBrief struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Hostname   string    `json:"hostname"`
	Kind       string    `json:"kind"`
	Lifecycle  string    `json:"lifecycle"`
	Purpose    string    `json:"purpose"`
	Addresses  []string  `json:"addresses"`
	Revision   int       `json:"revision"`
	LastReport time.Time `json:"last_report"`
}

func BriefNode(n Node) NodeBrief {
	return NodeBrief{n.ID, n.Name, n.Slug, n.Hostname, n.Kind, n.Lifecycle, n.Purpose, n.Addresses, n.Revision, n.LastReport}
}

type FindingBucket struct {
	Assessment      string    `json:"assessment"`
	Remediation     string    `json:"remediation"`
	Workflow        string    `json:"workflow"`
	SuppressedUntil time.Time `json:"suppressed_until"`
	Count           int       `json:"count"`
}
type FindingSummary struct {
	NodeID  string          `json:"node_id"`
	Buckets []FindingBucket `json:"buckets"`
}

func SummarizeFindings(node string, findings []Finding) FindingSummary {
	groups := map[string]FindingBucket{}
	for _, f := range findings {
		key := f.Assessment + "/" + f.Remediation + "/" + f.Workflow + "/" + f.SuppressedUntil.Format(time.RFC3339Nano)
		b := groups[key]
		b.Assessment = f.Assessment
		b.Remediation = f.Remediation
		b.Workflow = f.Workflow
		b.SuppressedUntil = f.SuppressedUntil
		b.Count++
		groups[key] = b
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := FindingSummary{NodeID: node, Buckets: []FindingBucket{}}
	for _, key := range keys {
		out.Buckets = append(out.Buckets, groups[key])
	}
	return out
}
