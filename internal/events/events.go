package events

import "encoding/json"

type Type string

const (
	WorkflowStarted   Type = "workflow.started"
	WorkflowCompleted Type = "workflow.completed"
	WorkflowFailed    Type = "workflow.failed"
	ModelCompleted    Type = "model.completed"
	ToolRequested     Type = "tool.requested"
	ToolCompleted     Type = "tool.completed"
	ToolFailed        Type = "tool.failed"
	Log               Type = "log"
)

type Event struct {
	ID         string          `json:"id"`
	TS         int64           `json:"ts"`
	Type       Type            `json:"type"`
	WorkflowID string          `json:"workflowId,omitempty"`
	Input      string          `json:"input,omitempty"`
	Output     string          `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	Text       string          `json:"text,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Name       string          `json:"name,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Level      string          `json:"level,omitempty"`
	Message    string          `json:"message,omitempty"`
}
