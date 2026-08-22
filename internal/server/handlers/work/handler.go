// Package work serves the surface a team does its work on: issues and their
// comments, workflows, tasks and the runs that execute them, the conversations
// that start them, and the files and traces they leave behind.
//
// This is one package rather than four because the entities are one story. An
// issue is assigned to a workflow, a workflow step dispatches a task, a task
// run reports back into the conversation that asked for it, and the run's
// artifacts hang off the issue. Splitting them would not remove that coupling,
// it would turn it into four packages passing each other interfaces.
package work

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/server/handlers/runterminal"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/server/access"
	"github.com/gougoujiang/buildmax/internal/server/turnqueue"
	wsconn "github.com/gougoujiang/buildmax/internal/server/websocket"
	artifactsvc "github.com/gougoujiang/buildmax/internal/service/artifact"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/quota"
)

// RunOutputLister reads what a run produced. An interface because the store
// that answers it is assembled above this package.
type RunOutputLister interface {
	ListRunOutputsByConversation(ctx context.Context, conversationID string, taskID *string) ([]model.ArtifactWithTask, error)
	GetTaskRunOutputFiles(ctx context.Context, taskRunID string) ([]model.TaskRunArtifact, error)
}

type Config struct {
	JWTSecret string

	// Users is part of authentication, not work ownership: every user-facing
	// route must reject a disabled account before it reads the team's work.
	Users         model.UserStore
	Issues        model.IssueStore
	IssueComments model.IssueCommentStore
	Workflows     model.WorkflowStore
	Tasks         model.TaskStore
	TaskRuns      model.TaskRunStore
	Agents        model.AgentStore
	Teams         model.TeamStore
	Conversations model.ConversationStore
	Messages      model.ConversationMessageStore
	RunOutputs    RunOutputLister
	// LLMCalls reads the managed call ledger for one run. Nil leaves that
	// route answering 503, which is what a deployment with no database has.
	LLMCalls model.LLMCallStore

	PersistStorage   blob.PersistStorage
	RunOutputStorage blob.RunOutputStorage
	// Artifacts lets an issue show what its runs published. Nil means this
	// deployment has no artifact store, and an issue reports only run output.
	Artifacts     *artifactsvc.Service
	WorkspacesDir string

	Quota          *quota.Service
	TitleGenerator llm.TitleGenerator
	// ConversationLLM answers a Tier 1 turn. Nil leaves conversations unable to
	// run, which the routes report rather than assume.
	ConversationLLM llm.LLMClient
	Audit           *audit.Recorder

	// Hub streams a running task's output; Turns keeps one conversation to one
	// turn at a time. Both are server-scoped and shared with the socket, so
	// they arrive rather than being created here.
	Hub   wsconn.StreamHub
	Turns *turnqueue.Registry
	// OnTerminal closes out a run cancelled here, reaching the listeners a
	// worker's own report reaches.
	OnTerminal func(ctx context.Context, info model.TaskRunTerminalInfo)
}

type Handler struct{ cfg Config }

// New builds the work surface. A nil Hub or Turns gets one of its own, which
// is what the unified handler did: a deployment with nobody watching still has
// runs to stream and turns to serialize.
func New(cfg Config) *Handler {
	if cfg.Hub == nil {
		cfg.Hub = wsconn.NewStreamHub()
	}
	if cfg.Turns == nil {
		cfg.Turns = turnqueue.NewRegistry()
	}
	return &Handler{cfg: cfg}
}

func (h *Handler) guard() *access.Guard {
	return &access.Guard{
		JWTSecret: h.cfg.JWTSecret,
		Users:     h.cfg.Users,
		Teams:     h.cfg.Teams,
		Audit:     h.cfg.Audit,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	// Issues
	mux.HandleFunc("GET /api/teams/{team_id}/issues", h.listIssuesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues", h.createIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}", h.getIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}/flow", h.getIssueFlowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/issues/{issue_id}", h.patchIssueHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/issues/{issue_id}/comments", h.listIssueCommentsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/comments", h.createIssueCommentHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", h.patchIssueCommentHandler)
	mux.HandleFunc("DELETE /api/teams/{team_id}/issues/{issue_id}/comments/{comment_id}", h.deleteIssueCommentHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/agent-runs", h.createIssueAgentRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/issues/{issue_id}/workflow-runs", h.createIssueWorkflowRunHandler)

	// Workflows
	mux.HandleFunc("GET /api/teams/{team_id}/workflows", h.listWorkflowsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows", h.createWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}", h.getWorkflowHandler)
	mux.HandleFunc("PATCH /api/teams/{team_id}/workflows/{workflow_id}", h.patchWorkflowHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}/revisions", h.listWorkflowRevisionsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows/{workflow_id}/revisions/{revision}/restore", h.restoreWorkflowRevisionHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflows/{workflow_id}/runs", h.listWorkflowRunsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/workflows/{workflow_id}/runs", h.createWorkflowRunHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/workflow-runs/{workflow_run_id}", h.getWorkflowRunHandler)

	// Files
	mux.HandleFunc("POST /api/teams/{team_id}/upload", h.uploadHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files", h.filesTreeHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/files/{path...}", h.fileContentHandler)

	// Conversations
	mux.HandleFunc("GET /api/teams/{team_id}/conversations", h.listConversationsHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations", h.createConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/messages", h.getConversationMessagesHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/messages", h.addConversationMessageHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.listConversationTasksHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/conversations/{conversation_id}/tasks", h.createConversationTaskHandler)

	// Tasks and task runs
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}", h.getTaskHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/runs", h.createTaskRunHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/cancel", h.cancelTaskHandler)
	mux.HandleFunc("POST /api/teams/{team_id}/tasks/{task_id}/retry", h.retryTaskHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/conversation", h.getTaskConversationHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/stream", h.getChatStreamHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/tasks/{task_id}/artifacts", h.listTaskArtifactsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/items", h.listArtifactItemsHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/artifacts/content", h.artifactContentHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/trace", h.getTaskRunTraceHandler)
	mux.HandleFunc("GET /api/teams/{team_id}/task-runs/{task_run_id}/llm-calls", h.listTaskRunLLMCallsHandler)
}

// runAnnouncer closes out a run cancelled here, reaching the same listeners a
// worker's own report does.
func (h *Handler) runAnnouncer() *runterminal.Announcer {
	return &runterminal.Announcer{Runs: h.cfg.TaskRuns, Hub: h.cfg.Hub, On: h.cfg.OnTerminal}
}
