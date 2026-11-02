package agents

import "andrelas6/ai-agent-go/internal/tools"

func NewSupportAgent(model Model) *Agent {
	return NewAgent("", model,
		tools.SearchKnowledgeBase(),
		tools.ClassifyItem(),
		tools.DraftReply(),
		tools.SendReply(),
	)
}
