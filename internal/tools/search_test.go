package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchKnowledgeBaseSpec(t *testing.T) {
	checkSpec(t, SearchKnowledgeBase(), "searchKnowledgeBase", "Search the support knowledge base for relevant articles.", `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "minLength": 1, "description": "what to look up"}
		},
		"required": ["query"],
		"additionalProperties": false
	}`)
}

const (
	billingArticle = "Double charges are usually a duplicate authorization that drops off in 3–5 days. If it already settled, refund immediately."
	refundArticle  = "Refunds post in 5–10 business days. Pro accounts can be expedited."
	exportArticle  = "The Safari export failure is a known bug (TICKET-4412). Workaround: use Chrome or the CSV export."
	pricingArticle = "Team plans are $20/seat/mo with a volume discount at 25+ seats. For 50+ seats, send the pricing PDF."
)

func TestSearchKnowledgeBase(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"one match", "pricing for 50 seats", []string{pricingArticle}},
		{"match ignores case", "Safari EXPORT is broken", []string{exportArticle}},
		{"several matches in knowledge base order", "refund my billing", []string{billingArticle, refundArticle}},
		{"no match", "I was charged twice", []string{"No exact match — use your judgment."}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := json.Marshal(map[string]string{"query": tt.query})
			if err != nil {
				t.Fatal(err)
			}
			got, err := runTool(t, SearchKnowledgeBase(), string(args))
			if err != nil {
				t.Fatalf("searchKnowledgeBase: %v", err)
			}
			want, err := json.Marshal(map[string][]string{"articles": tt.want})
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("searchKnowledgeBase(%q) = %s, want %s", tt.query, got, want)
			}
		})
	}
}

func TestSearchKnowledgeBaseRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{"empty query", `{"query":""}`, `"query" is required`},
		{"missing query", `{}`, `"query" is required`},
		{"query is a list", `{"query":["billing"]}`, "cannot unmarshal array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTool(t, SearchKnowledgeBase(), tt.args)
			if err == nil {
				t.Fatalf("searchKnowledgeBase(%s) = %s, want an error", tt.args, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
