package app

import (
	"ainfra/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

// RelationSuggestions reads observations without changing the manual graph.
// An address or key is usable only when exactly one active node claims it.
func (s *Service) RelationSuggestions(ctx context.Context, p domain.Principal, node string) ([]domain.RelationSuggestion, error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	out := []domain.RelationSuggestion{}
	err := s.store.Read(ctx, func(tx Tx) error {
		nodes, err := tx.TopologyNodes()
		if err != nil {
			return err
		}
		relations, err := decodeList[domain.Relation](tx, "relation")
		if err != nil {
			return err
		}
		accepted := map[string]bool{}
		for _, r := range relations {
			accepted[r.ID] = true
		}
		keys, addresses := map[string]string{}, map[string]string{}
		bind := func(index map[string]string, value, id string) {
			if value == "" {
				return
			}
			if old, ok := index[value]; ok && old != id {
				index[value] = ""
			} else if !ok {
				index[value] = id
			}
		}
		for _, n := range nodes {
			if n.Lifecycle == "retired" {
				continue
			}
			for _, k := range n.WireGuardPublicKeys {
				bind(keys, k, n.ID)
			}
			for _, a := range n.Addresses {
				bind(addresses, topologyAddress(a), n.ID)
			}
		}
		found := map[string]domain.RelationSuggestion{}
		add := func(source, target, kind, name, evidence string, at time.Time) {
			if target == "" || target == source {
				return
			}
			if kind == "tunnel_peer" && source > target {
				source, target = target, source
			}
			if node != "" && source != node && target != node {
				return
			}
			id := hash([]string{source, target, kind, name})
			if accepted[id] {
				return
			}
			v := domain.RelationSuggestion{ID: id, Source: source, Target: target, Type: kind, Name: name, Provenance: "observed", Evidence: evidence, ObservedAt: at}
			if old, ok := found[id]; !ok || at.After(old.ObservedAt) {
				found[id] = v
			}
		}
		fresh := func(v domain.Section) bool {
			return v.Status == "ok" && v.Complete && !v.ObservedAt.IsZero() && s.now().Sub(v.ObservedAt) <= 24*time.Hour && v.ObservedAt.Sub(s.now()) <= 5*time.Minute
		}
		for _, n := range nodes {
			if n.Lifecycle == "retired" {
				continue
			}
			if fresh(n.WireGuard) {
				local, peers := wireGuardTopology(n.WireGuard.Data)
				for iface, remote := range peers {
					own := local[iface]
					if keys[own] != n.ID {
						continue
					}
					for _, peer := range remote {
						pair := []string{own, peer}
						sort.Strings(pair)
						add(n.ID, keys[peer], "tunnel_peer", "WireGuard "+strings.Join(pair, " ↔ "), "Confirmed public keys; observed interface "+iface+" on "+n.ID, n.WireGuard.ObservedAt)
					}
				}
			}
			if fresh(n.Network) {
				var data map[string]json.RawMessage
				if json.Unmarshal(n.Network.Data, &data) != nil {
					continue
				}
				for _, source := range []struct {
					key, family string
					router      bool
				}{{"routes", "IPv4", false}, {"routes_ipv6", "IPv6", false}, {"ip/route", "IPv4", true}, {"ipv6/route", "IPv6", true}} {
					var routes []map[string]any
					if json.Unmarshal(data[source.key], &routes) != nil {
						continue
					}
					for _, r := range routes {
						if truth(r["disabled"]) || r["active"] == false || r["active"] == "false" {
							continue
						}
						gateway, iface, dst, table := field(r, "gateway"), field(r, "dev"), field(r, "dst"), field(r, "table")
						if source.router {
							dst = field(r, "dst-address")
							table = field(r, "routing-table")
							gateway, iface, _ = strings.Cut(gateway, "%")
						} else if table == "" {
							table = "main"
						}
						if table == "" {
							table = "unknown"
						}
						if iface == "" {
							iface = "unknown"
						}
						if dst == "" {
							continue
						}
						target := addresses[topologyAddress(gateway)]
						name := fmt.Sprintf("%s table=%s interface=%s destination=%s gateway=%s", source.family, table, iface, dst, gateway)
						add(n.ID, target, "uses_gateway", name, "Observed route on "+n.ID+"; gateway matches a unique manual node address", n.Network.ObservedAt)
					}
				}
			}
		}
		for _, v := range found {
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return nil
	})
	return out, err
}

func topologyAddress(v string) string {
	if p, e := netip.ParsePrefix(v); e == nil {
		return p.Addr().Unmap().String()
	}
	if a, e := netip.ParseAddr(v); e == nil {
		return a.WithZone("").Unmap().String()
	}
	return ""
}
func field(v map[string]any, key string) string {
	if x, ok := v[key]; ok && x != nil {
		return fmt.Sprint(x)
	}
	return ""
}
func truth(v any) bool { return v == true || v == "true" }

func wireGuardTopology(raw json.RawMessage) (map[string]string, map[string][]string) {
	local, peers := map[string]string{}, map[string][]string{}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return local, peers
	}
	var lines string
	if json.Unmarshal(data["public-key"], &lines) == nil {
		for _, line := range strings.Split(lines, "\n") {
			f := strings.Fields(line)
			if len(f) == 2 {
				local[f[0]] = f[1]
			}
		}
	}
	if json.Unmarshal(data["peers"], &lines) == nil {
		for _, line := range strings.Split(lines, "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				peers[f[0]] = append(peers[f[0]], f[1:]...)
			}
		}
	}
	var rows []map[string]any
	if json.Unmarshal(data["interface/wireguard"], &rows) == nil {
		for _, r := range rows {
			if !truth(r["disabled"]) {
				local[field(r, "name")] = field(r, "public-key")
			}
		}
	}
	if json.Unmarshal(data["interface/wireguard/peers"], &rows) == nil {
		for _, r := range rows {
			if !truth(r["disabled"]) {
				iface := field(r, "interface")
				peers[iface] = append(peers[iface], field(r, "public-key"))
			}
		}
	}
	return local, peers
}
