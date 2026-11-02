package tools

type draftArgs struct {
	ItemID  string `json:"itemId"`
	Message string `json:"message"`
}

type draftResult struct {
	OK      bool   `json:"ok"`
	DraftID string `json:"draftId"`
}

func DraftReply() Tool {
	schema := objectSchema(map[string]any{
		"itemId":  plainString(),
		"message": plainString(),
	}, "itemId", "message")

	return Tool{
		Name:        "draftReply",
		Description: "Write a draft reply for a work item. Does not send anything.",
		Parameters:  schema,
		Run:         strict(schema, draft),
	}
}

func draft(args draftArgs) (any, error) {
	return draftResult{OK: true, DraftID: "draft-" + args.ItemID}, nil
}
