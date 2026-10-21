package app

import (
	"ainfra/internal/domain"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func RandomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func digest(secret string) string {
	v := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(v[:])
}
func (s *Service) IssueToken(ctx context.Context, actor, p domain.Principal) (secret string, meta domain.Token, err error) {
	if actor.Role != "admin" {
		return "", meta, ErrForbidden
	}
	if !contains([]string{"admin", "reader", "editor", "agent", "probe"}, p.Role) {
		return "", meta, ErrInvalid
	}
	if (p.Role == "agent" && (len(p.Nodes) != 1 || p.Instance == "")) || (p.Role == "probe" && (len(p.Nodes) == 0 || p.Instance == "")) {
		return "", meta, ErrInvalid
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	secret = hex.EncodeToString(b)
	p.ID = s.id()
	meta = domain.Token{ID: p.ID, Principal: p, CreatedAt: s.now()}
	err = s.store.Write(ctx, func(tx Tx) error {
		for _, id := range p.Nodes {
			var n domain.Node
			if e := tx.Get("node", id, &n); e != nil {
				return e
			}
			if n.Instance != "" && n.Instance != p.Instance {
				return ErrConflict
			}
			n.Instance = p.Instance
			if e := tx.Put("node", id, n); e != nil {
				return e
			}
		}
		if e := tx.Put("token", digest(secret), meta); e != nil {
			return e
		}
		keys := map[string]string{}
		if e := tx.Get("token-index", "all", &keys); e != nil && e != ErrNotFound {
			return e
		}
		keys[meta.ID] = digest(secret)
		if e := tx.Put("token-index", "all", keys); e != nil {
			return e
		}
		return s.event(tx, actor, "token.issued", meta.ID, p.Role)
	})
	return
}
func (s *Service) Authenticate(ctx context.Context, secret string) (p domain.Principal, err error) {
	if len(secret) != 64 {
		return p, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error {
		var t domain.Token
		if e := tx.Get("token", digest(secret), &t); e != nil {
			return ErrForbidden
		}
		if t.Revoked {
			return ErrForbidden
		}
		p = t.Principal
		return nil
	})
	return
}
func (s *Service) Tokens(ctx context.Context, p domain.Principal) (out []domain.Token, err error) {
	if p.Role != "admin" {
		return nil, ErrForbidden
	}
	err = s.store.Read(ctx, func(tx Tx) error { var e error; out, e = decodeList[domain.Token](tx, "token"); return e })
	return
}

// Token ID index permits revocation without recovering the credential.
func (s *Service) RevokeToken(ctx context.Context, p domain.Principal, id string) error {
	if p.Role != "admin" {
		return ErrForbidden
	}
	return s.store.Write(ctx, func(tx Tx) error {
		var keys map[string]string
		if e := tx.Get("token-index", "all", &keys); e != nil && e != ErrNotFound {
			return e
		}
		key := keys[id]
		if key == "" {
			return ErrNotFound
		}
		var t domain.Token
		if e := tx.Get("token", key, &t); e != nil {
			return e
		}
		t.Revoked = true
		if e := tx.Put("token", key, t); e != nil {
			return e
		}
		return s.event(tx, p, "token.revoked", id, "")
	})
}

func (s *Service) Reenroll(ctx context.Context, p domain.Principal, node, instance string, revision int) error {
	if p.Role != "admin" {
		return ErrForbidden
	}
	if instance == "" || len(instance) > 100 {
		return ErrInvalid
	}
	return s.store.Write(ctx, func(tx Tx) error {
		var n domain.Node
		if e := tx.Get("node", node, &n); e != nil {
			return e
		}
		if n.Revision != revision {
			return ErrConflict
		}
		keys := map[string]string{}
		if e := tx.Get("token-index", "all", &keys); e != nil && e != ErrNotFound {
			return e
		}
		for _, key := range keys {
			var token domain.Token
			if e := tx.Get("token", key, &token); e != nil {
				return e
			}
			if contains(token.Principal.Nodes, node) {
				token.Revoked = true
				if e := tx.Put("token", key, token); e != nil {
					return e
				}
			}
		}
		n.Instance = instance
		n.Sequence = 0
		n.Revision++
		if e := tx.Put("node", node, n); e != nil {
			return e
		}
		return s.event(tx, p, "node.reenrolled", node, instance)
	})
}
