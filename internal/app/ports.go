package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrForbidden = errors.New("forbidden")
var ErrConflict = errors.New("conflict")
var ErrNotFound = errors.New("not_found")
var ErrInvalid = errors.New("invalid_input")

type Tx interface {
	Get(kind, id string, out any) error
	Put(kind, id string, value any) error
	Delete(kind, id string) error
	List(kind string) ([]json.RawMessage, error)
}
type Store interface {
	Read(context.Context, func(Tx) error) error
	Write(context.Context, func(Tx) error) error
	Search(context.Context, string, bool, int, int) ([]json.RawMessage, error)
}
type Service struct {
	store Store
	now   func() time.Time
	id    func() string
}

func New(store Store, now func() time.Time, id func() string) *Service {
	return &Service{store: store, now: now, id: id}
}
func decodeList[T any](tx Tx, kind string) ([]T, error) {
	raw, err := tx.List(kind)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raw))
	for _, b := range raw {
		var v T
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
