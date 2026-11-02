package tools

import (
	"context"
	"encoding/json"
	"errors"
)

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Run         func(ctx context.Context, args json.RawMessage) (any, error)
}

var errNotBuiltYet = errors.New("not built yet")

func notBuiltYet(ctx context.Context, args json.RawMessage) (any, error) {
	return nil, errNotBuiltYet
}
