package ui

import (
	"ainfra/internal/domain"
	"sort"
)

type hostingOption struct{ ID, Name string }
type hostingFilterOptions struct {
	Providers []string
	Hosts     []hostingOption
}

func hostingFilters(nodes []domain.Node, relations []domain.Relation) hostingFilterOptions {
	h := domain.NewHostingIndex(nodes, relations)
	out := hostingFilterOptions{}
	providers := map[string]bool{}
	for id, n := range h.Nodes {
		for _, p := range h.Providers(id) {
			providers[p] = true
		}
		if len(h.Parents[id]) == 0 || len(h.Children[id]) > 0 {
			out.Hosts = append(out.Hosts, hostingOption{ID: id, Name: n.Name})
		}
	}
	for p := range providers {
		out.Providers = append(out.Providers, p)
	}
	sort.Strings(out.Providers)
	sort.Slice(out.Hosts, func(i, j int) bool {
		if out.Hosts[i].Name == out.Hosts[j].Name {
			return out.Hosts[i].ID < out.Hosts[j].ID
		}
		return out.Hosts[i].Name < out.Hosts[j].Name
	})
	return out
}
