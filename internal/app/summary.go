package app

import (
	"ainfra/internal/domain"
	"context"
)

func putNode(tx Tx, n domain.Node) error {
	if e := tx.Put("node", n.ID, n); e != nil {
		return e
	}
	return tx.Put("node-summary", n.ID, domain.SummarizeNode(n))
}

func (s *Service) NodeView(ctx context.Context, p domain.Principal, id, view string) (any, error) {
	if view != "" && view != "brief" && view != "summary" && view != "full" {
		return nil, ErrInvalid
	}
	n, err := s.Node(ctx, p, id)
	if err != nil {
		return nil, err
	}
	n = domain.NormalizeNodeCounters(n)
	switch view {
	case "full":
		return n, nil
	case "brief":
		return domain.BriefNode(n), nil
	default:
		return domain.SummarizeNode(n), nil
	}
}

func (s *Service) NodesView(ctx context.Context, p domain.Principal, view string) (any, error) {
	if view == "full" {
		return s.FullNodes(ctx, p)
	}
	if view != "" && view != "brief" && view != "summary" {
		return nil, ErrInvalid
	}
	nodes, err := s.Nodes(ctx, p)
	if err != nil {
		return nil, err
	}
	if view == "summary" {
		return nodes, nil
	}
	out := make([]domain.NodeBrief, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, domain.BriefNode(n))
	}
	return out, nil
}

// FullNodes is reserved for explicit exports and reassessment, not list views.
func (s *Service) FullNodes(ctx context.Context, p domain.Principal) (out []domain.Node, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Node](tx, "node"); return e })
	for i := range out {
		out[i] = domain.NormalizeNodeCounters(out[i])
	}
	return
}

func refreshFindingSummary(tx Tx, node string) error {
	findings, e := decodeNodeList[domain.Finding](tx, "finding", node)
	if e != nil {
		return e
	}
	return tx.Put("finding-summary", node, domain.SummarizeFindings(node, findings))
}
func (s *Service) findingSummaries(ctx context.Context) (out map[string]domain.FindingSummary, err error) {
	out = map[string]domain.FindingSummary{}
	err = s.store.Read(ctx, func(tx Tx) error {
		rows, e := decodeList[domain.FindingSummary](tx, "finding-summary")
		if e != nil {
			return e
		}
		for _, r := range rows {
			out[r.NodeID] = r
		}
		return nil
	})
	return
}
