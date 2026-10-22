package ui

import "sort"

// Each provider owns a vertical band. Within it, each primary hosting subtree
// occupies a contiguous interval, so guests from different hosts never interleave.
// Extra DAG parents remain visible as edges; each node is still drawn only once.
type mapGroup struct {
	ID, Name         string
	Y, Height, Count int
	External         bool
}

func layoutMap(nodes map[string]mapNode, edges []mapEdge) ([]mapGroup, []mapEdge) {
	depth, indegree := map[string]int{}, map[string]int{}
	children := map[string][]string{}
	for id := range nodes {
		indegree[id] = 0
	}
	for _, e := range edges {
		if e.Type == "runs_on" || e.Placement {
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
	less := func(a, b string) bool {
		if nodes[a].Name != nodes[b].Name {
			return nodes[a].Name < nodes[b].Name
		}
		return a < b
	}
	parents := map[string]string{}
	selectParent := func(child, parent string) {
		if depth[parent] >= depth[child] {
			return
		} // Includes malformed legacy cycles.
		previous, ok := parents[child]
		if !ok || depth[parent] > depth[previous] || depth[parent] == depth[previous] && less(parent, previous) {
			parents[child] = parent
		}
	}
	for _, e := range edges {
		if e.Type == "runs_on" || e.Placement {
			selectParent(e.Source, e.Target)
		}
	}
	// Unattached legacy cycles still get a provider band, without trusting their
	// cyclic hosting links as a tree or hiding any of the original relations.
	for id, n := range nodes {
		if n.Provider || n.Unmanaged || parents[id] != "" {
			continue
		}
		const unknown = "provider:UNKNOWN"
		if _, ok := nodes[unknown]; !ok {
			nodes[unknown] = mapNode{ID: unknown, Name: "UNKNOWN", Kind: "Хостер / площадка", Services: "Размещение не определено", Provider: true}
		}
		parents[id] = unknown
		depth[id] = 1
		edges = append(edges, mapEdge{Source: id, Target: unknown, SourceName: n.Name, TargetName: "UNKNOWN", Type: "hosted_by", Name: "Размещение не определено", Label: relationLabel("hosted_by"), Placement: true})
	}
	children = map[string][]string{}
	for child, parent := range parents {
		children[parent] = append(children[parent], child)
	}
	for id := range children {
		sort.Slice(children[id], func(i, j int) bool { return less(children[id][i], children[id][j]) })
	}
	roots := []string{}
	for id, n := range nodes {
		if n.Provider {
			roots = append(roots, id)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return less(roots[i], roots[j]) })
	heights := map[string]int{}
	var measure func(string) int
	measure = func(id string) int {
		height := 0
		for i, child := range children[id] {
			if i > 0 {
				height += 48
			}
			height += measure(child)
		}
		if height < 130 {
			height = 130
		}
		heights[id] = height
		return height
	}
	groups := []mapGroup{}
	var place func(string, string, int)
	place = func(id, group string, top int) {
		n := nodes[id]
		n.X = 32 + depth[id]*340
		n.Y = top + (heights[id]-130)/2
		n.Group = group
		nodes[id] = n
		childTop := top
		for _, child := range children[id] {
			place(child, group, childTop)
			childTop += heights[child] + 48
		}
	}
	top := 16
	for _, id := range roots {
		height := measure(id)
		place(id, id, top+64)
		count := 0
		for _, n := range nodes {
			if n.Group == id && !n.Provider && !n.Unmanaged {
				count++
			}
		}
		groups = append(groups, mapGroup{ID: id, Name: nodes[id].Name, Y: top, Height: height + 96, Count: count})
		top += height + 96 + 200
	}
	external := []string{}
	for id, n := range nodes {
		if n.Unmanaged {
			external = append(external, id)
		}
	}
	sort.Slice(external, func(i, j int) bool { return less(external[i], external[j]) })
	if len(external) > 0 {
		const group = "external-wireguard-peers"
		for i, id := range external {
			n := nodes[id]
			n.X = 32
			n.Y = top + 64 + i*178
			n.Group = group
			nodes[id] = n
		}
		groups = append(groups, mapGroup{ID: group, Name: "Внешние WireGuard peers — размещение неизвестно", Y: top, Height: len(external)*178 - 48 + 96, Count: len(external), External: true})
	}
	return groups, edges
}
