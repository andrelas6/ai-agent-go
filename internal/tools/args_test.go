package tools

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestArgsStructsMatchTheirSchemas(t *testing.T) {
	tests := []struct {
		tool Tool
		args reflect.Type
	}{
		{SearchKnowledgeBase(), reflect.TypeFor[searchArgs]()},
		{ClassifyItem(), reflect.TypeFor[classifyArgs]()},
		{DraftReply(), reflect.TypeFor[draftArgs]()},
		{SendReply(), reflect.TypeFor[sendArgs]()},
	}

	for _, tt := range tests {
		t.Run(tt.tool.Name, func(t *testing.T) {
			var tags []string
			for field := range tt.args.Fields() {
				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				tags = append(tags, name)
			}
			slices.Sort(tags)

			properties, _ := tt.tool.Parameters["properties"].(map[string]any)
			want := slices.Sorted(maps.Keys(properties))

			if !slices.Equal(tags, want) {
				t.Errorf("%s json tags = %v, schema properties = %v", tt.args.Name(), tags, want)
			}
		})
	}
}
