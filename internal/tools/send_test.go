package tools

import "testing"

func TestSendReplySpec(t *testing.T) {
	checkSpec(t, SendReply(), "sendReply", "Send the drafted reply to the customer. This really emails them.", `{
		"type": "object",
		"properties": {
			"itemId": {"type": "string", "minLength": 1},
			"draftId": {"type": "string", "minLength": 1}
		},
		"required": ["itemId", "draftId"],
		"additionalProperties": false
	}`)
}
