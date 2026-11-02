package tools

import (
	"strings"
	"testing"
)

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

func TestDraftReply(t *testing.T) {
	got, err := runTool(t, DraftReply(), `{"itemId":"item-3","message":"Here is our pricing PDF."}`)
	if err != nil {
		t.Fatalf("draftReply: %v", err)
	}
	if want := `{"ok":true,"draftId":"draft-item-3"}`; got != want {
		t.Errorf("draftReply = %s, want %s", got, want)
	}
}

func TestDraftReplyRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{"missing message", `{"itemId":"item-1"}`, `"message" is missing`},
		{"empty message", `{"itemId":"item-1","message":""}`, `"message" is empty`},
		{"missing itemId", `{"message":"hi"}`, `"itemId" is missing`},
		{"unknown field", `{"itemId":"item-1","message":"hi","to":"x@y.z"}`, `unknown field "to"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTool(t, DraftReply(), tt.args)
			if err == nil {
				t.Fatalf("draftReply(%s) = %s, want an error", tt.args, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
