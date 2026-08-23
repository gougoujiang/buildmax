package conversation

import (
	"context"

	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
)

// ConversationResult is the output of processing one turn. The engine may return
// a direct reply and/or one or more spawned task_run_ids.
type ConversationResult struct {
	// Reply is an optional direct reply to the user (e.g. clarification or acknowledgment).
	Reply string
	// Runs are the Tier 2 runs this turn spawned, if any.
	//
	// Each carries both identifiers rather than the run's alone. A caller that
	// reports one to a client needs to say which task it belongs to, and two
	// parallel slices are how those drift apart.
	Runs []SpawnedRun
}

// SpawnedRun is one Tier 2 run and the task it belongs to.
type SpawnedRun struct {
	TaskID string
	RunID  string
}

const (
	ChannelPortal     = convchannel.ChannelPortal
	ChannelTelegram   = convchannel.ChannelTelegram
	ChannelCron       = convchannel.ChannelCron
	ChannelWebhook    = convchannel.ChannelWebhook
	ChannelSystem     = convchannel.ChannelSystem
	ChannelWorkflow   = convchannel.ChannelWorkflow
	ChannelIssueAgent = convchannel.ChannelIssueAgent
)

// SyntheticChannels are conversations nobody holds; see convchannel.
var SyntheticChannels = convchannel.SyntheticChannels

var ValidChannels = convchannel.ValidChannels

func ValidChannel(ch string) bool {
	return convchannel.ValidChannel(ch)
}

// TurnEngine processes one turn; may create Tier 2 runs and/or return a direct reply.
type TurnEngine interface {
	Process(ctx context.Context, conversationID, taskID string, turn convchannel.Turn) (ConversationResult, error)
}
