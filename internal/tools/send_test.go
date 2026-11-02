package tools

import (
	"strings"
	"testing"
)

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

func TestSendReply(t *testing.T) {
	got, err := runTool(t, SendReply(), `{"itemId":"item-3","draftId":"draft-item-3"}`)
	if err != nil {
		t.Fatalf("sendReply: %v", err)
	}
	if want := `{"sent":true,"itemId":"item-3","draftId":"draft-item-3"}`; got != want {
		t.Errorf("sendReply = %s, want %s", got, want)
	}
}

func TestSendReplyRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{"missing draftId", `{"itemId":"item-1"}`, `"draftId" is required`},
		{"null draftId", `{"itemId":"item-1","draftId":null}`, `"draftId" is required`},
		{"missing itemId", `{"draftId":"draft-item-1"}`, `"itemId" is required`},
		{"draftId is a number", `{"itemId":"item-1","draftId":7}`, "cannot unmarshal number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTool(t, SendReply(), tt.args)
			if err == nil {
				t.Fatalf("sendReply(%s) = %s, want an error", tt.args, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
