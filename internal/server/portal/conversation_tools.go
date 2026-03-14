package portal

import (
	"context"
	"fmt"

	"buildmax/internal/conversation/adapter"
	"buildmax/internal/storage/entity"
	"buildmax/internal/tools"
)

func (h *Handler) startChatRunner(workspaceID, userID, conversationID string) tools.StartChatRunner {
	if h.cfg.ChatStore == nil {
		return nil
	}
	return adapter.NewStartChatRunner(h.doStartChat, workspaceID, userID, conversationID)
}

func (h *Handler) doStartChat(ctx context.Context, workspaceID, userID, inputVal string, agentID *string, conversationID *string) (chatID, runID string, err error) {
	var input string
	if agentID != nil && *agentID != "" {
		if h.cfg.AgentStore == nil {
			return "", "", fmt.Errorf("agents not configured")
		}
		agent, err := h.cfg.AgentStore.GetAgent(ctx, *agentID)
		if err != nil {
			return "", "", err
		}
		if agent == nil || agent.WorkspaceID != workspaceID {
			return "", "", fmt.Errorf("agent not found or not in workspace")
		}
		if inputVal != "" {
			input = inputVal
		} else {
			input = buildChatInputFromAgent(agent, "")
		}
	} else {
		input = inputVal
	}
	title := truncateChatTitle(input, 50)
	titlePromptTokens, titleCompletionTokens := 0, 0
	if h.cfg.ChatTitleGenerator != nil {
		if gen, usage, err := h.cfg.ChatTitleGenerator.GenerateChatTitle(ctx, input); err == nil && gen != "" {
			title = gen
			titlePromptTokens = usage.PromptTokens
			titleCompletionTokens = usage.CompletionTokens
		}
	}
	if h.cfg.QuotaChecker != nil {
		allowed, reason := h.cfg.QuotaChecker.Check(ctx, userID, 1, titlePromptTokens+titleCompletionTokens)
		if !allowed {
			return "", "", fmt.Errorf("quota exceeded: %s", reason)
		}
	}
	chat, err := h.cfg.ChatStore.CreateChat(ctx, &entity.CreateChatInput{
		WorkspaceID:           workspaceID,
		Input:                 input,
		Title:                 title,
		CreatedBy:             userID,
		TitlePromptTokens:     titlePromptTokens,
		TitleCompletionTokens: titleCompletionTokens,
		AgentID:               agentID,
		ConversationID:        conversationID,
	})
	if err != nil {
		return "", "", err
	}
	runIDVal := ""
	if chat.LastRunID != nil {
		runIDVal = *chat.LastRunID
	}
	return chat.ChatID, runIDVal, nil
}
