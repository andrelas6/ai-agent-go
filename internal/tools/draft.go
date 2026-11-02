package tools

func DraftReply() Tool {
	return Tool{
		Name:        "draftReply",
		Description: "Write a draft reply for a work item. Does not send anything.",
		Parameters: objectSchema(map[string]any{
			"itemId":  plainString(),
			"message": plainString(),
		}, "itemId", "message"),
		Run: notBuiltYet,
	}
}
