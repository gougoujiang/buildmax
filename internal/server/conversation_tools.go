// Conversation tools: server-side implementation of StartChatFunc for the conversation agent.
package server

import (
	"context"
	"fmt"

	"buildmax/internal/core"
	"buildmax/internal/tools"
)

// startChatFunc returns a StartChatFunc that creates a chat (and first run) using server config.
func (s *Server) startChatFunc(workspaceID, userID string) tools.StartChatFunc {
	if s.cfg.ChatStore == nil {
		return nil
	}
	return func(ctx context.Context, input string, agentID *string) (chatID, runID string, err error) {
		return s.doStartChat(ctx, workspaceID, userID, input, agentID)
	}
}

// doStartChat creates a chat and its first run (same logic as POST .../chats). Used by start_chat tool.
func (s *Server) doStartChat(ctx context.Context, workspaceID, userID, inputVal string, agentID *string) (chatID, runID string, err error) {
	var input string
	if agentID != nil && *agentID != "" {
		if s.cfg.AgentStore == nil {
			return "", "", fmt.Errorf("agents not configured")
		}
		agent, err := s.cfg.AgentStore.GetAgent(ctx, *agentID)
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
	if s.cfg.ChatTitleGenerator != nil {
		if gen, usage, err := s.cfg.ChatTitleGenerator.GenerateChatTitle(ctx, input); err == nil && gen != "" {
			title = gen
			titlePromptTokens = usage.PromptTokens
			titleCompletionTokens = usage.CompletionTokens
		}
	}
	if s.cfg.QuotaChecker != nil {
		allowed, reason := s.cfg.QuotaChecker.Check(ctx, userID, 1, titlePromptTokens+titleCompletionTokens)
		if !allowed {
			return "", "", fmt.Errorf("quota exceeded: %s", reason)
		}
	}
	chat, err := s.cfg.ChatStore.CreateChat(ctx, workspaceID, input, title, userID, titlePromptTokens, titleCompletionTokens, agentID)
	if err != nil {
		return "", "", err
	}
	runIDVal := ""
	if chat.LastRunID != nil {
		runIDVal = *chat.LastRunID
	}
	return chat.ChatID, runIDVal, nil
}

// conversationToolsForRequest returns the tool list for the conversation loop (default + start_chat when ChatStore is set).
func (s *Server) conversationToolsForRequest(workspaceID, userID string) []core.Tool {
	return tools.BuildConversationToolsWithStartChat(workspaceID, userID, s.startChatFunc(workspaceID, userID))
}
