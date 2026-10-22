package app

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Topology returns the complete compact graph, independent of list pagination.
// Observed suggestions and unmanaged keys never become manual inventory entries.
func (s *Service) Topology(ctx context.Context, p domain.Principal) (out domain.Topology, err error) {
	if !reader(p) {
		return out, ErrForbidden
	}
	if out.Nodes, err = s.Nodes(ctx, p); err != nil {
		return
	}
	if out.Relations, err = s.Relations(ctx, p); err != nil {
		return
	}
	if out.Suggestions, err = s.RelationSuggestions(ctx, p, ""); err != nil {
		return
	}
	out.UnmanagedPeers = []domain.WireGuardPeer{}
	err = s.store.Read(ctx, func(tx Tx) error {
		nodes, e := tx.TopologyNodes()
		if e != nil {
			return e
		}
		// Even an ambiguous key is already assigned; do not invent a new host.
		assigned := map[string]bool{}
		for _, n := range nodes {
			if n.Lifecycle != "retired" {
				for _, key := range n.WireGuardPublicKeys {
					assigned[key] = true
				}
			}
		}
		for _, n := range nodes {
			wg := n.WireGuard
			if n.Lifecycle == "retired" || wg.Status != "ok" || !wg.Complete || wg.ObservedAt.IsZero() || s.now().Sub(wg.ObservedAt) > 24*time.Hour || wg.ObservedAt.After(s.now().Add(5*time.Minute)) {
				continue
			}
			_, peers := wireGuardTopology(wg.Data)
			for iface, keys := range peers {
				seen := map[string]bool{}
				for _, key := range keys {
					if key != "" && !assigned[key] && !seen[key] {
						out.UnmanagedPeers = append(out.UnmanagedPeers, domain.WireGuardPeer{NodeID: n.ID, Interface: iface, PublicKey: key, ObservedAt: wg.ObservedAt})
						seen[key] = true
					}
				}
			}
		}
		return nil
	})
	sort.Slice(out.UnmanagedPeers, func(i, j int) bool {
		a, b := out.UnmanagedPeers[i], out.UnmanagedPeers[j]
		return a.NodeID+"/"+a.Interface+"/"+a.PublicKey < b.NodeID+"/"+b.Interface+"/"+b.PublicKey
	})
	return
}

func (s *Service) contextPublicServices(ctx context.Context, p domain.Principal, root domain.Node, relations []domain.Relation, b *strings.Builder) error {
	nodes, err := s.Nodes(ctx, p)
	if err != nil {
		return err
	}
	children := map[string][]string{}
	for _, r := range relations {
		if r.Type == "runs_on" {
			children[r.Target] = append(children[r.Target], r.Source)
		}
	}
	hosted := map[string]bool{}
	queue := []string{root.ID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if hosted[id] {
			continue
		}
		hosted[id] = true
		queue = append(queue, children[id]...)
	}
	b.WriteString("## Internet-facing services, including hosted descendants (untrusted)\n\n")
	count := 0
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	for _, n := range nodes {
		if !hosted[n.ID] || n.Lifecycle == "retired" {
			continue
		}
		for _, service := range n.Services {
			if service.Exposure != "public" {
				continue
			}
			if count >= 100 {
				b.WriteString("> [truncated; fetch topology and full node services]\n\n")
				return nil
			}
			raw, _ := json.Marshal(struct {
				NodeID   string         `json:"node_id"`
				NodeName string         `json:"node_name"`
				Service  domain.Service `json:"service"`
			}{n.ID, n.Name, service})
			fmt.Fprintf(b, "> %s\n", raw)
			count++
		}
	}
	if count == 0 {
		b.WriteString("> No internet-facing services documented; exposure is unconfirmed.\n")
	}
	b.WriteString("\n")
	return nil
}
