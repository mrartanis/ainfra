package ui

import (
	"ainfra/internal/domain"
	"ainfra/internal/domain/version"
	"fmt"
	"sort"
	"strings"
)

type mapNode struct {
	ID, Name, Address, Services, Kind, Group string
	X, Y                                     int
	Unmanaged, Provider                      bool
	Router, EntryPoint                       bool
}
type mapEdge struct {
	Source, Target, SourceName, TargetName, Type, Name, Path, Label string
	Observed, Placement                                             bool
}
type mapView struct {
	Nodes                                                []mapNode
	Groups                                               []mapGroup
	Edges                                                []mapEdge
	Width, Height, PublicCount, NodeCount, ProviderCount int
	RouterCount, EntryPointCount                         int
	Filter                                               string
}

func packageState(p map[string]any) string {
	installed, _ := p["version"].(string)
	candidate, _ := p["candidate"].(string)
	cmp, err := version.Compare(candidate, installed)
	if err != nil {
		return "Кандидат неизвестен"
	}
	if cmp > 0 {
		return "Доступно обновление"
	}
	if cmp < 0 {
		return "Кандидат старее установленного"
	}
	return "Совпадает с кандидатом"
}

func shortMapText(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return s
}

func buildMap(topology domain.Topology, filter string) mapView {
	if filter != "runs_on" && filter != "tunnel_peer" {
		filter = "all"
	}
	out := mapView{Filter: filter, Nodes: []mapNode{}, Edges: []mapEdge{}}
	nodes := map[string]mapNode{}
	for _, n := range topology.Nodes {
		if n.Lifecycle == "retired" {
			continue
		}
		services := []string{}
		for _, service := range n.Services {
			if service.Exposure == "public" {
				services = append(services, service.Name)
				out.PublicCount++
			}
		}
		label := strings.Join(services, ", ")
		if label == "" {
			label = "Внешние сервисы не описаны"
		}
		kind := map[string]string{"physical": "Хост", "vm": "Виртуальная машина", "container": "Контейнер", "router": "Маршрутизатор", "other": "Узел"}[n.Kind]
		router, entry := n.Kind == "router", n.Labels["entry_point"] == "true"
		if router {
			out.RouterCount++
		}
		if entry {
			out.EntryPointCount++
		}
		nodes[n.ID] = mapNode{ID: n.ID, Name: n.Name, Address: strings.Join(n.Addresses, ", "), Kind: kind, Services: label, Router: router, EntryPoint: entry}
	}
	out.NodeCount = len(nodes)
	hosting := domain.NewHostingIndex(topology.Nodes, topology.Relations)
	for id := range hosting.Nodes {
		if len(hosting.Parents[id]) != 0 {
			continue
		}
		provider := hosting.Providers(id)[0]
		pid := "provider:" + provider
		nodes[pid] = mapNode{ID: pid, Name: provider, Kind: "Хостер / площадка", Services: "Размещение хостов", Provider: true}
	}
	edges := []mapEdge{}
	add := func(source, target, kind, name string, observed bool) {
		a, okA := nodes[source]
		b, okB := nodes[target]
		if !okA || !okB {
			return
		}
		edges = append(edges, mapEdge{Source: source, Target: target, SourceName: a.Name, TargetName: b.Name, Type: kind, Name: name, Observed: observed, Label: relationLabel(kind)})
	}
	for _, r := range topology.Relations {
		add(r.Source, r.Target, r.Type, r.Name, false)
	}
	for id := range hosting.Nodes {
		if len(hosting.Parents[id]) == 0 {
			add(id, "provider:"+hosting.Providers(id)[0], "hosted_by", "По размещению хоста", false)
			edges[len(edges)-1].Placement = true
		}
	}
	for _, r := range topology.Suggestions {
		if r.Type != "tunnel_peer" {
			continue
		}
		duplicate := false
		for _, e := range edges {
			if e.Type == r.Type && (e.Source == r.Source && e.Target == r.Target || e.Source == r.Target && e.Target == r.Source) {
				duplicate = true
			}
		}
		if !duplicate {
			add(r.Source, r.Target, r.Type, r.Name, true)
		}
	}
	for _, peer := range topology.UnmanagedPeers {
		id := "peer:" + peer.PublicKey
		nodes[id] = mapNode{ID: id, Name: "WireGuard peer", Address: shortMapText(peer.PublicKey, 32), Kind: "Вне инвентаря", Services: "Узел не привязан к public key", Unmanaged: true}
		add(peer.NodeID, id, "tunnel_peer", "Интерфейс "+peer.Interface+" · "+peer.ObservedAt.Format("2006-01-02 15:04 UTC"), true)
	}
	out.Groups, edges = layoutMap(nodes, edges)
	for i := range out.Groups {
		for _, n := range nodes {
			if n.Group == out.Groups[i].ID && n.EntryPoint {
				out.Groups[i].EntryPointCount++
			}
		}
	}
	ids := []string{}
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := nodes[ids[i]], nodes[ids[j]]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.X != b.X {
			return a.X < b.X
		}
		return a.ID < b.ID
	})
	for _, id := range ids {
		n := nodes[id]
		if n.Provider {
			out.ProviderCount++
		}
		out.Nodes = append(out.Nodes, n)
		if out.Width < n.X+306 {
			out.Width = n.X + 306
		}
	}
	groups := map[string]mapGroup{}
	for _, g := range out.Groups {
		groups[g.ID] = g
		if out.Height < g.Y+g.Height+16 {
			out.Height = g.Y + g.Height + 16
		}
	}
	contentWidth, lane := out.Width, 0
	// Filters change the visible connections without moving the hosting layout.
	for _, e := range edges {
		if filter != "all" && e.Type != filter && !(filter == "runs_on" && e.Placement) {
			continue
		}
		a, b := nodes[e.Source], nodes[e.Target]
		if a.Group != b.Group {
			// Cross-provider links travel through the gaps between columns and
			// bands, then a dedicated outer lane; they never cut through cards.
			gutter := contentWidth + 32 + lane*16
			lane++
			e.Path = fmt.Sprintf("M %d %d H %d V %d H %d V %d H %d V %d H %d", a.X+270, a.Y+65, a.X+292, groups[a.Group].Y+groups[a.Group].Height-12, gutter, groups[b.Group].Y+46, b.X+292, b.Y+65, b.X+270)
			if out.Width < gutter+32 {
				out.Width = gutter + 32
			}
		} else {
			x1, y1, x2, y2 := a.X+270, a.Y+65, b.X, b.Y+65
			if a.X > b.X {
				x1 = a.X
				x2 = b.X + 270
			}
			if a.X == b.X {
				x1 = a.X + 270
				x2 = b.X + 270
				e.Path = fmt.Sprintf("M %d %d C %d %d %d %d %d %d", x1, y1, x1+28, y1, x2+28, y2, x2, y2)
			} else {
				mid := (x1 + x2) / 2
				e.Path = fmt.Sprintf("M %d %d C %d %d %d %d %d %d", x1, y1, mid, y1, mid, y2, x2, y2)
			}
		}
		out.Edges = append(out.Edges, e)
	}
	if out.Width < 720 {
		out.Width = 720
	}
	if out.Height < 260 {
		out.Height = 260
	}
	return out
}

func relationLabel(kind string) string {
	switch kind {
	case "hosted_by":
		return "Размещён у"
	case "runs_on":
		return "Запущен на"
	case "tunnel_peer":
		return "WireGuard"
	case "uses_gateway":
		return "Шлюз"
	case "backs_up_to":
		return "Бэкап в"
	case "depends_on":
		return "Зависит от"
	}
	return kind
}
