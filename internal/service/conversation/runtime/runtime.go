package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	convtool "github.com/gougoujiang/buildmax/internal/service/conversation/tool"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

const (
	maxIterations         = 10
	internalSystemChannel = "system"
)

const systemPromptBase = `You are the user's assistant. You coordinate between the user and background tasks. Reply concisely.

# Decision order
First evaluate whether the user's request should continue an existing task (use ContinueTask) rather than creating a new one (StartTask). When the user refers to an existing task (e.g. "add to that task", "try again", "what about the last run?"), prefer ContinueTask. Use ListTasks/GetTask to decide when needed.

# Tools
- StartTask: create and schedule a new background task (long-running job, analysis). Tell the user you have started a task and will report back when it completes. Do not provide internal task or run IDs. Do not tell the user to check a task detail page — you will deliver the result directly.
- ListTasks: list recent tasks in the current conversation (up to 10). Use when the user asks what tasks they have or for recent activity.
- GetTask: get detail for one task by task_id. Use when the user asks about a specific task's status or result.
- ContinueTask: add a follow-up message to an existing task (new run). Use when the user wants to continue, retry, or add to an existing task.

When starting or continuing a task, tell the user you are working on it and will get back to them with results. Do not expose internal IDs.

# Task results
When you receive a message starting with "[Task Result]", a background task has completed. Read the status and output, then:
- If succeeded: summarize the result clearly and concisely for the user. Present key findings naturally.
- If failed: explain what went wrong and suggest next steps (e.g. retry, provide more info).
Do not mention task IDs, run IDs, or internal system details. Speak to the user as their assistant.`

func currentSystemPrompt() string {
	return systemPromptBase + "\n\nToday's date: " + time.Now().Format("2006-01-02") + "."
}

// TurnRunInput configures one conversation turn execution.
type TurnRunInput struct {
	ConversationID string
	Message        string
	Channel        string
	UserID         string
	TeamID         string
	TaskService    *task.TaskService
	AgentSummaries []convtool.AgentSummary
	TitleGenerator llm.TitleGenerator
	StreamSink     llm.StreamSink
}

func buildConversationTools(in TurnRunInput) []llm.Tool {
	if in.Channel == internalSystemChannel || in.TaskService == nil {
		return nil
	}
	svc := in.TaskService
	tools := []llm.Tool{
		convtool.NewStartTaskTool(
			convtool.NewStartTaskServiceRunner(svc, in.ConversationID, in.TeamID, in.UserID),
			in.AgentSummaries,
		),
	}
	if r := convtool.NewListTasksStoreRunner(svc.Tasks); r != nil {
		tools = append(tools, convtool.NewListTasksTool(in.ConversationID, r))
	}
	if r := convtool.NewGetTaskStoreRunner(svc.Tasks); r != nil {
		tools = append(tools, convtool.NewGetTaskTool(in.ConversationID, r))
	}
	if r := convtool.NewContinueTaskServiceRunner(svc); r != nil {
		tools = append(tools, convtool.NewContinueTaskTool(in.ConversationID, in.UserID, r))
	}
	return tools
}

type conversationBuffer struct {
	ctx            context.Context
	conversationID string
	msgStore       model.ConversationMessageStore
	msgs           []llm.Message
}

func (b *conversationBuffer) HistoryMessages() []llm.Message {
	return b.msgs
}

func (b *conversationBuffer) Append(m llm.Message) error {
	b.msgs = append(b.msgs, m)
	var toolCallID *string
	if m.Role == "tool" && m.ToolCallID != "" {
		toolCallID = &m.ToolCallID
	}
	var toolCallsJSON *string
	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		js, err := marshalToolCalls(m.ToolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCallsJSON = js
	}
	// Reasoning state is persisted so a turn that resumes from the stored
	// conversation carries what the upstream protocol needs back. A failure to
	// encode it drops it rather than failing the turn: the run continues
	// without reasoning continuity, which is what a protocol without it does
	// anyway.
	var providerStateJSON *string
	if m.Role == "assistant" && m.ProviderState != nil {
		if encoded, err := json.Marshal(m.ProviderState); err == nil {
			js := string(encoded)
			providerStateJSON = &js
		}
	}
	// Non-text content is stored the same way and for the same reason: a Tier 1
	// turn replays from these rows, so an image a tool returned has to survive
	// or the next turn discusses something it can no longer see.
	var partsJSON *string
	if len(m.Parts) > 0 {
		if encoded, err := json.Marshal(m.Parts); err == nil {
			js := string(encoded)
			partsJSON = &js
		}
	}
	_, err := b.msgStore.AppendMessage(b.ctx, model.AppendMessageInput{
		ConversationID:    b.conversationID,
		Role:              m.Role,
		Content:           m.Content,
		ToolCallID:        toolCallID,
		ToolCallsJSON:     toolCallsJSON,
		ProviderStateJSON: providerStateJSON,
		PartsJSON:         partsJSON,
	})
	return err
}

func replayMessageFromStore(m model.ConversationMessage) llm.Message {
	toolCallID := ""
	if m.ToolCallID != nil {
		toolCallID = *m.ToolCallID
	}
	role := m.Role
	// backward compat: old data stored system-channel messages with role "system"
	if role == internalSystemChannel {
		role = "user"
	}
	msg := llm.Message{Role: role, Content: m.Content, ToolCallID: toolCallID}
	if m.ToolCallsJSON != nil && *m.ToolCallsJSON != "" {
		var toolCalls []llm.ToolCall
		if err := json.Unmarshal([]byte(*m.ToolCallsJSON), &toolCalls); err == nil {
			msg.ToolCalls = toolCalls
		}
	}
	if m.ProviderStateJSON != nil && *m.ProviderStateJSON != "" {
		var state llm.ProviderState
		if err := json.Unmarshal([]byte(*m.ProviderStateJSON), &state); err == nil {
			msg.ProviderState = &state
		}
	}
	if m.PartsJSON != nil && *m.PartsJSON != "" {
		var parts []llm.ContentPart
		if err := json.Unmarshal([]byte(*m.PartsJSON), &parts); err == nil {
			msg.Parts = parts
		}
	}
	return msg
}

type preparedRun struct {
	firstRound bool
	buffer     *conversationBuffer
	toolsList  []llm.Tool
}

func prepareRun(ctx context.Context, msgStore model.ConversationMessageStore, in TurnRunInput) (*preparedRun, error) {
	msgs, err := msgStore.ListMessages(ctx, in.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	firstRound := len(msgs) == 0
	channelPtr := &in.Channel
	if _, err := msgStore.AppendMessage(ctx, model.AppendMessageInput{
		ConversationID: in.ConversationID,
		Role:           "user",
		Content:        in.Message,
		Channel:        channelPtr,
	}); err != nil {
		return nil, fmt.Errorf("append incoming message: %w", err)
	}
	llmMsgs := make([]llm.Message, 0, len(msgs)+2)
	for _, m := range msgs {
		llmMsgs = append(llmMsgs, replayMessageFromStore(m))
	}
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: in.Message})

	return &preparedRun{
		firstRound: firstRound,
		buffer: &conversationBuffer{
			ctx:            ctx,
			conversationID: in.ConversationID,
			msgStore:       msgStore,
			msgs:           llmMsgs,
		},
		toolsList: buildConversationTools(in),
	}, nil
}

func executeRun(ctx context.Context, llmClient llm.LLMClient, in TurnRunInput, prepared *preparedRun) (string, error) {
	tools := llm.NewToolRegistry()
	tools.AppendTools(prepared.toolsList...)

	reply, _, err := agent.RunLoop(ctx, agent.RunLoopOpts{
		LLMClient:    llmClient,
		SystemPrompt: currentSystemPrompt(),
		ToolRegistry: tools,
		MaxIter:      maxIterations,
		History:      prepared.buffer,
		StreamSink:   in.StreamSink,
		Policy:       agentapp.NewNonInteractivePolicy(),
	})
	if err != nil {
		return "", err
	}
	return reply, nil
}

func maybeUpdateTitle(ctx context.Context, convStore model.ConversationStore, in TurnRunInput, prepared *preparedRun) {
	if !prepared.firstRound || in.Message == "" || in.TitleGenerator == nil {
		return
	}
	if title, _, _, err := in.TitleGenerator.GenerateTitle(ctx, in.Message); err == nil && title != "" {
		_ = convStore.UpdateConversationTitle(ctx, in.ConversationID, title)
	}
}

func runLoop(ctx context.Context, convStore model.ConversationStore, msgStore model.ConversationMessageStore, llmClient llm.LLMClient, in TurnRunInput) (string, error) {
	prepared, err := prepareRun(ctx, msgStore, in)
	if err != nil {
		return "", err
	}
	reply, err := executeRun(ctx, llmClient, in, prepared)
	if err != nil {
		return "", err
	}
	maybeUpdateTitle(ctx, convStore, in, prepared)
	return reply, nil
}

// Run executes one conversation turn. Streaming is enabled when in.StreamSink is non-nil.
func Run(ctx context.Context, convStore model.ConversationStore, msgStore model.ConversationMessageStore, llmClient llm.LLMClient, in TurnRunInput) (string, error) {
	if llmClient == nil {
		if in.StreamSink != nil {
			return "", fmt.Errorf("conversation stream LLM not configured")
		}
		return "", fmt.Errorf("conversation LLM not configured")
	}
	return runLoop(ctx, convStore, msgStore, llmClient, in)
}

func marshalToolCalls(toolCalls []llm.ToolCall) (*string, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(toolCalls)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
