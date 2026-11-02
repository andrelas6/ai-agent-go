package tools

func SearchKnowledgeBase() Tool {
	return Tool{
		Name:        "searchKnowledgeBase",
		Description: "Search the support knowledge base for relevant articles.",
		Parameters: objectSchema(map[string]any{
			"query": describedString("what to look up"),
		}, "query"),
		Run: notBuiltYet,
	}
}
