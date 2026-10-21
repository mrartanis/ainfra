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

// FullNodes is reserved for explicit exports and reassessment, not list views.
func (s *Service) FullNodes(ctx context.Context, p domain.Principal) (out []domain.Node, err error) {
	if !reader(p) {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Node](tx, "node"); return e })
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
