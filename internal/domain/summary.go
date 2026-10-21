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
			data.PackageCount = len(data.Packages)
			data.UnknownOrigins = 0
			data.Updates = 0
			for _, p := range data.Packages {
				if p.Origin == "" || p.SourceVersion == "" {
					data.UnknownOrigins++
				}
				if c, e := version.Compare(p.Candidate, p.Version); e == nil && c > 0 {
					data.Updates++
				}
			}
			data.Packages = nil
			packages.Data, _ = json.Marshal(data)
			observations["packages"] = packages
		}
	}
	n.Observations = observations
	return n
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
