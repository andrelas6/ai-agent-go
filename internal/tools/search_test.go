package tools

import "testing"

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
