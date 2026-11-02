package tools

import "strings"

type article struct {
	key  string
	text string
}

var knowledgeBase = []article{
	{"billing", "Double charges are usually a duplicate authorization that drops off in 3–5 days. If it already settled, refund immediately."},
	{"refund", "Refunds post in 5–10 business days. Pro accounts can be expedited."},
	{"export", "The Safari export failure is a known bug (TICKET-4412). Workaround: use Chrome or the CSV export."},
	{"pricing", "Team plans are $20/seat/mo with a volume discount at 25+ seats. For 50+ seats, send the pricing PDF."},
}

const noMatch = "No exact match — use your judgment."

type searchArgs struct {
	Query string `json:"query"`
}

type searchResult struct {
	Articles []string `json:"articles"`
}

func SearchKnowledgeBase() Tool {
	schema := objectSchema(map[string]any{
		"query": describedString("what to look up"),
	}, "query")

	return Tool{
		Name:        "searchKnowledgeBase",
		Description: "Search the support knowledge base for relevant articles.",
		Parameters:  schema,
		Run:         decodeStringArgs(schema, search),
	}
}

func search(args searchArgs) (any, error) {
	query := strings.ToLower(args.Query)
	var hits []string
	for _, a := range knowledgeBase {
		if strings.Contains(query, a.key) {
			hits = append(hits, a.text)
		}
	}
	if len(hits) == 0 {
		hits = []string{noMatch}
	}
	return searchResult{Articles: hits}, nil
}
