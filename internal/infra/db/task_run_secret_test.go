package db

import (
	"testing"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// TestRecordEnvGrant_Idempotent proves the audit write records one row per
// (run, secret, item) even when the worker retries the secrets fetch.
func TestRecordEnvGrant_Idempotent(t *testing.T) {
	s, ctx := newTestStore(t)
	userID := newTestUser(t, s, "grant-audit")
	teamID := newTestTeam(t, s, userID)

	agent, err := s.CreateAgentInTeam(ctx, agentdef.CreateInput{
		TeamID: teamID, UserID: userID, Def: agentdef.Definition{Name: "deployer"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	sec, err := s.CreateSecret(ctx, coresecret.CreateInput{
		TeamID: teamID, Name: "gh", CreatedBy: userID, ItemNames: []string{"token"},
		Sealed: coresecret.Sealed{Ciphertext: []byte("c"), Nonce: []byte("n"), WrappedDEK: []byte("w"), KeyID: "file:root:1"},
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	t.Cleanup(func() {
		s.db.Where("team_id IN (SELECT id FROM team WHERE public_id = ?)", teamID).Delete(&secretRow{})
	})

	conversation, err := s.CreateConversation(ctx, userID, "portal", userID)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{ConversationID: conversation.ID, Input: "in", CreatedBy: userID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	runID := *task.LastRunID

	rec := coresecret.GrantRecord{
		TaskRunID: runID, SecretID: sec.ID, ItemName: "token",
		AgentID: agent.ID, AgentRevision: 1, EnvName: "GH_TOKEN",
	}
	if err := s.RecordEnvGrant(ctx, rec); err != nil {
		t.Fatalf("RecordEnvGrant: %v", err)
	}
	// A retry re-records the same materialization once.
	if err := s.RecordEnvGrant(ctx, rec); err != nil {
		t.Fatalf("RecordEnvGrant retry: %v", err)
	}

	runKey, _ := lookupKey(ctx, s.db, "task_run", runID)
	var count int64
	if err := s.db.WithContext(ctx).Model(&taskRunSecretRow{}).
		Where("task_run_id = ?", runKey).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("recorded %d rows, want 1 (idempotent)", count)
	}

	var row taskRunSecretRow
	if err := s.db.WithContext(ctx).Where("task_run_id = ?", runKey).Take(&row).Error; err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.Delivery != "env" || row.EnvName != "GH_TOKEN" || row.Status != "materialized" || row.MaterializedAt == nil {
		t.Fatalf("row = %+v", row)
	}
}
