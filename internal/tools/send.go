package tools

func SendReply() Tool {
	return Tool{
		Name:        "sendReply",
		Description: "Send the drafted reply to the customer. This really emails them.",
		Parameters: objectSchema(map[string]any{
			"itemId":  plainString(),
			"draftId": plainString(),
		}, "itemId", "draftId"),
		Run: notBuiltYet,
	}
}
