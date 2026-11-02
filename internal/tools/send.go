package tools

type sendArgs struct {
	ItemID  string `json:"itemId"`
	DraftID string `json:"draftId"`
}

type sendResult struct {
	Sent    bool   `json:"sent"`
	ItemID  string `json:"itemId"`
	DraftID string `json:"draftId"`
}

func SendReply() Tool {
	schema := objectSchema(map[string]any{
		"itemId":  plainString(),
		"draftId": plainString(),
	}, "itemId", "draftId")

	return Tool{
		Name:        "sendReply",
		Description: "Send the drafted reply to the customer. This really emails them.",
		Parameters:  schema,
		Run:         decodeStringArgs(schema, send),
	}
}

func send(args sendArgs) (any, error) {
	return sendResult{Sent: true, ItemID: args.ItemID, DraftID: args.DraftID}, nil
}
