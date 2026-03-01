// Conversation tools: server-side StartChatRunner for the conversation agent.
package server

import (
	"context"
	"fmt"

	"buildmax/internal/tools"
)

// serverStartChatRunner implements tools.StartChatRunner using server config (ChatStore, etc.).
type serverStartChatRunner struct {
	s              *Server
	workspaceID    string
	userID         string
	conversationID string
}

func (r *serverStartChatRunner) StartChat(ctx context.Context, _, _, input string, agentID *string) (chatID, runID string, err error) {
	var convID *string
	if r.conversationID != "" {
		convID = &r.conversationID
	}
	return r.s.doStartChat(ctx, r.workspaceID, r.userID, input, agentID, convID)
}

// startChatRunner returns a StartChatRunner when ChatStore is set; otherwise nil (StartChat tool is not added).
// conversationID is the Tier 1 conversation id when the runner is used from the conversation flow; empty for other uses.
func (s *Server) startChatRunner(workspaceID, userID, conversationID string) tools.StartChatRunner {
	if s.cfg.ChatStore == nil {
		return nil
	}
	return &serverStartChatRunner{s: s, workspaceID: workspaceID, userID: userID, conversationID: conversationID}
}

// doStartChat creates a chat and its first run (same logic as POST .../chats). Used by start_chat tool.
// conversationID is optional; when set (e.g. from Tier 1), the chat record stores it.
func (s *Server) doStartChat(ctx context.Context, workspaceID, userID, inputVal string, agentID *string, conversationID *string) (chatID, runID string, err error) {
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
	chat, err := s.cfg.ChatStore.CreateChat(ctx, workspaceID, input, title, userID, titlePromptTokens, titleCompletionTokens, agentID, conversationID)
	if err != nil {
		return "", "", err
	}
	runIDVal := ""
	if chat.LastRunID != nil {
		runIDVal = *chat.LastRunID
	}
	return chat.ChatID, runIDVal, nil
}
