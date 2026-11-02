package tools

import "testing"

func TestDraftReplySpec(t *testing.T) {
	checkSpec(t, DraftReply(), "draftReply", "Write a draft reply for a work item. Does not send anything.", `{
		"type": "object",
		"properties": {
			"itemId": {"type": "string", "minLength": 1},
			"message": {"type": "string", "minLength": 1}
		},
		"required": ["itemId", "message"],
		"additionalProperties": false
	}`)
}
