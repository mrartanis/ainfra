package domain

import (
	"sort"
	"strings"
)

const UnknownProvider = "UNKNOWN"

// HostingIndex uses only runs_on: network and backup relations do not imply
// placement. Descendants inherit their root host's provider.
type HostingIndex struct {
	Nodes    map[string]Node
	Parents  map[string][]string
	Children map[string][]string
}

func NewHostingIndex(nodes []Node, relations []Relation) HostingIndex {
	h := HostingIndex{Nodes: map[string]Node{}, Parents: map[string][]string{}, Children: map[string][]string{}}
	for _, n := range nodes {
		if n.Lifecycle != "retired" {
			h.Nodes[n.ID] = n
		}
	}
	for _, r := range relations {
		_, source := h.Nodes[r.Source]
		_, target := h.Nodes[r.Target]
		if r.Type == "runs_on" && source && target {
			h.Parents[r.Source] = append(h.Parents[r.Source], r.Target)
			h.Children[r.Target] = append(h.Children[r.Target], r.Source)
		}
	}
	return h
}

func (h HostingIndex) Descendants(host string) map[string]bool {
	out := map[string]bool{}
	if _, ok := h.Nodes[host]; !ok {
		return out
	}
	queue := []string{host}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if out[id] {
			continue
		}
		out[id] = true
		queue = append(queue, h.Children[id]...)
	}
	return out
}

func (h HostingIndex) Providers(id string) []string {
	seen, providers := map[string]bool{}, map[string]bool{}
	queue := []string{id}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		n, ok := h.Nodes[id]
		if !ok {
			continue
		}
		if len(h.Parents[id]) != 0 {
			queue = append(queue, h.Parents[id]...)
			continue
		}
		provider := strings.TrimSpace(n.Labels["provider"])
		if provider == "" {
			provider = UnknownProvider
		}
		providers[provider] = true
	}
	if len(providers) == 0 {
		providers[UnknownProvider] = true
	}
	out := []string{}
	for p := range providers {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
