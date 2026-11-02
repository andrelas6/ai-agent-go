package tools

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeStringArgsPanicsOnABrokenSchema(t *testing.T) {
	tests := []struct {
		name      string
		schema    map[string]any
		wantPanic string
	}{
		{"no properties", map[string]any{"type": "object", "required": []string{"itemId"}}, "properties"},
		{"no required list", map[string]any{"type": "object", "properties": map[string]any{"itemId": plainString()}}, "required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				got := recover()
				if got == nil {
					t.Fatal("decodeStringArgs built a tool from a broken schema, want a panic")
				}
				if !strings.Contains(fmt.Sprint(got), tt.wantPanic) {
					t.Errorf("panic = %v, want it to mention %q", got, tt.wantPanic)
				}
			}()
			decodeStringArgs(tt.schema, func(args struct{}) (any, error) { return nil, nil })
		})
	}
}
