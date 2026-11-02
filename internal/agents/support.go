package agents

import "andrelas6/ai-agent-go/internal/tools"

const SupportPrompt = `You are a support triage agent.

For each work item the user gives you:
1. Classify it with classifyItem.
2. Search the knowledge base with searchKnowledgeBase if it helps.
3. Draft a reply with draftReply.
4. Send the reply with sendReply.

Work through every item, then briefly summarize what you did.`

func NewSupportAgent(model Model) *Agent {
	return NewAgent(SupportPrompt, model,
		tools.SearchKnowledgeBase(),
		tools.ClassifyItem(),
		tools.DraftReply(),
		tools.SendReply(),
	)
}
