package tools

import (
	"context"
	"encoding/json"
)

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Run         func(ctx context.Context, args json.RawMessage) (any, error)
}
