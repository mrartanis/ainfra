package ui

import (
	"ainfra/internal/domain"
	"ainfra/internal/domain/version"
	"fmt"
	"sort"
	"strings"
)

type mapNode struct {
	ID, Name, Address, Services, Kind string
	X, Y                              int
	Unmanaged                         bool
}
type mapEdge struct {
	Source, Target, SourceName, TargetName, Type, Name, Path, Label string
	Observed                                                        bool
}
type mapView struct {
	Nodes                      []mapNode
	Edges                      []mapEdge
	Width, Height, PublicCount int
	Filter                     string
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
		nodes[n.ID] = mapNode{ID: n.ID, Name: n.Name, Address: strings.Join(n.Addresses, ", "), Kind: kind, Services: label}
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
	// Longest path through the hosting DAG preserves arbitrary nesting and
	// multiple parents. A malformed legacy cycle cannot trap the layout.
	depth, indegree := map[string]int{}, map[string]int{}
	children := map[string][]string{}
	for id := range nodes {
		indegree[id] = 0
	}
	for _, e := range edges {
		if e.Type == "runs_on" {
			children[e.Target] = append(children[e.Target], e.Source)
			indegree[e.Source]++
		}
	}
	queue := []string{}
	for id, n := range indegree {
		if n == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, child := range children[id] {
			if depth[child] < depth[id]+1 {
				depth[child] = depth[id] + 1
			}
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	for _, e := range edges {
		if nodes[e.Target].Unmanaged && depth[e.Target] < depth[e.Source]+1 {
			depth[e.Target] = depth[e.Source] + 1
		}
	}
	ids := []string{}
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if depth[ids[i]] != depth[ids[j]] {
			return depth[ids[i]] < depth[ids[j]]
		}
		a, b := nodes[ids[i]], nodes[ids[j]]
		if a.Name == b.Name {
			return a.ID < b.ID
		}
		return a.Name < b.Name
	})
	rows := map[int]int{}
	for _, id := range ids {
		n := nodes[id]
		n.X = 32 + depth[id]*340
		n.Y = 48 + rows[depth[id]]*190
		rows[depth[id]]++
		nodes[id] = n
		out.Nodes = append(out.Nodes, n)
		if out.Width < n.X+306 {
			out.Width = n.X + 306
		}
		if out.Height < n.Y+170 {
			out.Height = n.Y + 170
		}
	}
	// Filters change the visible connections without moving the hosting layout.
	for _, e := range edges {
		if filter != "all" && e.Type != filter {
			continue
		}
		a, b := nodes[e.Source], nodes[e.Target]
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
