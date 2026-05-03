package db

import "buildmax/internal/core/model"

type userRow struct {
	ID                uint    `gorm:"primaryKey;autoIncrement"`
	UserID            string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	Email             string  `gorm:"type:varchar(255);uniqueIndex;not null"`
	Name              string  `gorm:"type:varchar(255)"`
	QuotaTier         string  `gorm:"type:varchar(64)"`
	LastLoginAt       *int64  `gorm:""`
	LastLoginPlatform *string `gorm:"type:varchar(32)"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
}

func (userRow) TableName() string { return "user" }

type teamRow struct {
	ID                uint    `gorm:"primaryKey;autoIncrement"`
	TeamID            string  `gorm:"column:team_id;type:varchar(64);uniqueIndex;not null"`
	Name              string  `gorm:"type:varchar(255);not null"`
	PersonalForUserID *string `gorm:"column:personal_for_user_id;type:varchar(64);uniqueIndex"`
	QuotaTier         string  `gorm:"column:quota_tier;type:varchar(64)"`
	CreatedBy         string  `gorm:"type:varchar(64);not null"`
	CreatedAt         int64   `gorm:"autoCreateTime"`
	UpdatedAt         int64   `gorm:"autoUpdateTime"`
}

func (teamRow) TableName() string { return "team" }

type teamMemberRow struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	TeamID    string `gorm:"column:team_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user"`
	UserID    string `gorm:"column:user_id;type:varchar(64);not null;uniqueIndex:uq_team_member_team_user"`
	Role      string `gorm:"type:varchar(32);not null"`
	CreatedAt int64  `gorm:"autoCreateTime"`
}

func (teamMemberRow) TableName() string { return "team_member" }

type workflowRow struct {
	ID          uint   `gorm:"primaryKey;autoIncrement"`
	WorkflowID  string `gorm:"column:workflow_id;type:varchar(64);uniqueIndex;not null"`
	TeamID      string `gorm:"column:team_id;type:varchar(64);not null;index"`
	Name        string `gorm:"type:varchar(255);not null"`
	Description string `gorm:"type:text;not null"`
	Definition  string `gorm:"type:longtext;not null"`
	Status      string `gorm:"type:varchar(32);not null;default:'draft'"`
	CreatedBy   string `gorm:"type:varchar(64);not null"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
}

func (workflowRow) TableName() string { return "workflow" }

type workflowRunRow struct {
	ID             uint    `gorm:"primaryKey;autoIncrement"`
	WorkflowRunID  string  `gorm:"column:workflow_run_id;type:varchar(64);uniqueIndex;not null"`
	WorkflowID     string  `gorm:"column:workflow_id;type:varchar(64);not null;index"`
	IssueID        *string `gorm:"column:issue_id;type:varchar(64);index"`
	ConversationID string  `gorm:"column:conversation_id;type:varchar(64);not null;index"`
	Status         string  `gorm:"type:varchar(32);not null"`
	CreatedBy      string  `gorm:"type:varchar(64);not null"`
	CreatedAt      int64   `gorm:"autoCreateTime"`
	StartedAt      *int64  `gorm:""`
	EndedAt        *int64  `gorm:""`
	ErrorMessage   *string `gorm:"type:text"`
}

func (workflowRunRow) TableName() string { return "workflow_run" }

type workflowStepRunRow struct {
	ID            uint    `gorm:"primaryKey;autoIncrement"`
	StepRunID     string  `gorm:"column:workflow_step_run_id;type:varchar(64);uniqueIndex;not null"`
	WorkflowRunID string  `gorm:"column:workflow_run_id;type:varchar(64);not null;index"`
	StepID        string  `gorm:"column:step_id;type:varchar(128);not null"`
	StepIndex     int     `gorm:"column:step_index;not null"`
	StepType      string  `gorm:"column:step_type;type:varchar(32);not null"`
	TargetAgentID *string `gorm:"column:target_agent_id;type:varchar(64);index"`
	Prompt        string  `gorm:"type:text;not null"`
	Status        string  `gorm:"type:varchar(32);not null"`
	TaskID        *string `gorm:"column:task_id;type:varchar(64);index"`
	TaskRunID     *string `gorm:"column:task_run_id;type:varchar(64);index"`
	OutputSummary *string `gorm:"type:text"`
	ErrorMessage  *string `gorm:"type:text"`
	CreatedAt     int64   `gorm:"autoCreateTime"`
	StartedAt     *int64  `gorm:""`
	EndedAt       *int64  `gorm:""`
}

func (workflowStepRunRow) TableName() string { return "workflow_step_run" }

type issueRow struct {
	ID           uint    `gorm:"primaryKey;autoIncrement"`
	IssueID      string  `gorm:"column:issue_id;type:varchar(64);uniqueIndex;not null"`
	UserID       string  `gorm:"type:varchar(64);not null;index"`
	TeamID       string  `gorm:"type:varchar(64);index"`
	Title        string  `gorm:"type:varchar(255);not null"`
	Description  string  `gorm:"type:text;not null"`
	Status       string  `gorm:"type:varchar(32);not null"`
	AssigneeKind *string `gorm:"type:varchar(32)"`
	AssigneeID   *string `gorm:"type:varchar(64)"`
	CreatedBy    string  `gorm:"type:varchar(64);not null"`
	CreatedAt    int64   `gorm:"autoCreateTime"`
	UpdatedAt    int64   `gorm:"autoUpdateTime"`
}

func (issueRow) TableName() string { return "issue" }

type taskRow struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement"`
	TaskID                string  `gorm:"column:task_id;type:varchar(64);uniqueIndex;not null"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index"`
	TeamID                string  `gorm:"type:varchar(64);index"`
	IssueID               *string `gorm:"column:issue_id;type:varchar(64);index"`
	Status                string  `gorm:"type:varchar(32);not null"`
	Input                 string  `gorm:"type:text;not null"`
	Title                 string  `gorm:"type:varchar(256)"`
	TitlePromptTokens     int     `gorm:""`
	TitleCompletionTokens int     `gorm:""`
	Output                *string `gorm:"type:text"`
	CreatedBy             string  `gorm:"type:varchar(64);not null"`
	CreatedAt             int64   `gorm:"autoCreateTime"`
	StartedAt             *int64  `gorm:""`
	EndedAt               *int64  `gorm:""`
	ErrorMessage          *string `gorm:"type:text"`
	SessionID             *string `gorm:"type:varchar(36)"`
	LastRunID             *string `gorm:"type:varchar(64);index"`
	AgentID               *string `gorm:"type:varchar(64);index"`
}

func (taskRow) TableName() string { return "task" }

type taskRunRow struct {
	ID               uint    `gorm:"primaryKey;autoIncrement"`
	TaskRunID        string  `gorm:"column:task_run_id;type:varchar(64);uniqueIndex;not null"`
	TaskID           string  `gorm:"column:task_id;type:varchar(64);not null;index"`
	Input            string  `gorm:"type:text;not null"`
	CreatedBy        string  `gorm:"type:varchar(64);index"`
	CreatedByType    string  `gorm:"type:varchar(32)"`
	TriggerSource    string  `gorm:"type:varchar(64)"`
	Status           string  `gorm:"type:varchar(32);not null"`
	Output           *string `gorm:"type:text"`
	ErrorMessage     *string `gorm:"type:text"`
	StartedAt        *int64  `gorm:""`
	EndedAt          *int64  `gorm:""`
	SessionID        *string `gorm:"type:varchar(36)"`
	WorkerType       string  `gorm:"type:varchar(32)"`
	K8sJobName       *string `gorm:"type:varchar(128)"`
	K8sJobCreatedAt  *int64  `gorm:"column:k8s_job_created_at"`
	PromptTokens     *int    `gorm:""`
	CompletionTokens *int    `gorm:""`
	CreatedAt        int64   `gorm:"autoCreateTime"`
}

func (taskRunRow) TableName() string { return "task_run" }

type taskRunArtifactRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	TaskRunID    string `gorm:"type:varchar(64);not null;uniqueIndex:uq_task_run_artifact_run_path"`
	RelativePath string `gorm:"type:varchar(512);not null;uniqueIndex:uq_task_run_artifact_run_path"`
}

func (taskRunArtifactRow) TableName() string { return "task_run_artifact" }

type agentRow struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	AgentID      string `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID       string `gorm:"type:varchar(64);not null;index"`
	TeamID       string `gorm:"type:varchar(64);index"`
	Name         string `gorm:"type:varchar(255);not null"`
	Description  string `gorm:"type:text"`
	Instructions string `gorm:"type:text"`
	CreatedAt    int64  `gorm:"autoCreateTime"`
}

func (agentRow) TableName() string { return "agent" }

type quotaTierRow struct {
	TierName           string `gorm:"column:tier_name;primaryKey;type:varchar(64)"`
	MaxRunsPerPeriod   int    `gorm:"column:max_runs_per_period;not null"`
	MaxTokensPerPeriod int    `gorm:"column:max_tokens_per_period;not null"`
	PeriodDays         int    `gorm:"column:period_days;not null"`
}

func (quotaTierRow) TableName() string { return "quota_tier" }

type conversationRow struct {
	ID             uint   `gorm:"primaryKey;autoIncrement"`
	ConversationID string `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID         string `gorm:"type:varchar(64);not null;index"`
	TeamID         string `gorm:"type:varchar(64);index"`
	Channel        string `gorm:"type:varchar(32);not null"`
	Title          string `gorm:"type:varchar(256)"`
	CreatedBy      string `gorm:"type:varchar(64);not null"`
	CreatedAt      int64  `gorm:"autoCreateTime"`
}

func (conversationRow) TableName() string { return "conversation" }

type conversationMessageRow struct {
	ID                    uint    `gorm:"primaryKey;autoIncrement"`
	ConversationMessageID string  `gorm:"type:varchar(64);uniqueIndex;not null"`
	ConversationID        string  `gorm:"type:varchar(64);not null;index"`
	Role                  string  `gorm:"type:varchar(16);not null"`
	Content               string  `gorm:"type:text;not null"`
	Channel               *string `gorm:"type:varchar(32)"`
	ToolCallID            *string `gorm:"type:varchar(64);column:tool_call_id"`
	ToolCallsJSON         *string `gorm:"type:text;column:tool_calls"`
	CreatedAt             int64   `gorm:"autoCreateTime"`
}

func (conversationMessageRow) TableName() string { return "conversation_message" }

type userWebhookKeyRow struct {
	ID        uint   `gorm:"primaryKey;autoIncrement"`
	KeyID     string `gorm:"type:varchar(64);uniqueIndex;not null"`
	UserID    string `gorm:"type:varchar(64);not null;index"`
	KeyHash   string `gorm:"type:varchar(128);not null;uniqueIndex"`
	Name      string `gorm:"type:varchar(255)"`
	CreatedAt int64  `gorm:"autoCreateTime"`
}

func (userWebhookKeyRow) TableName() string { return "user_webhook_key" }

func toModelUser(row *userRow) *model.User {
	if row == nil {
		return nil
	}
	return &model.User{
		ID:                row.ID,
		UserID:            row.UserID,
		Email:             row.Email,
		Name:              row.Name,
		QuotaTier:         row.QuotaTier,
		LastLoginAt:       row.LastLoginAt,
		LastLoginPlatform: row.LastLoginPlatform,
		CreatedAt:         row.CreatedAt,
	}
}

func toModelUsers(rows []userRow) []model.User {
	out := make([]model.User, len(rows))
	for i := range rows {
		out[i] = *toModelUser(&rows[i])
	}
	return out
}

func fromModelUser(m *model.User) *userRow {
	if m == nil {
		return nil
	}
	return &userRow{
		ID:                m.ID,
		UserID:            m.UserID,
		Email:             m.Email,
		Name:              m.Name,
		QuotaTier:         m.QuotaTier,
		LastLoginAt:       m.LastLoginAt,
		LastLoginPlatform: m.LastLoginPlatform,
		CreatedAt:         m.CreatedAt,
	}
}

func toModelTeam(row *teamRow) *model.Team {
	if row == nil {
		return nil
	}
	return &model.Team{
		ID:                row.ID,
		TeamID:            row.TeamID,
		Name:              row.Name,
		PersonalForUserID: row.PersonalForUserID,
		QuotaTier:         row.QuotaTier,
		CreatedBy:         row.CreatedBy,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func toModelTeams(rows []teamRow) []model.Team {
	out := make([]model.Team, len(rows))
	for i := range rows {
		out[i] = *toModelTeam(&rows[i])
	}
	return out
}

func fromModelTeam(m *model.Team) *teamRow {
	if m == nil {
		return nil
	}
	return &teamRow{
		ID:                m.ID,
		TeamID:            m.TeamID,
		Name:              m.Name,
		PersonalForUserID: m.PersonalForUserID,
		QuotaTier:         m.QuotaTier,
		CreatedBy:         m.CreatedBy,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func toModelTeamMember(row *teamMemberRow) *model.TeamMember {
	if row == nil {
		return nil
	}
	return &model.TeamMember{
		ID:        row.ID,
		TeamID:    row.TeamID,
		UserID:    row.UserID,
		Role:      row.Role,
		CreatedAt: row.CreatedAt,
	}
}

func toModelTeamMembers(rows []teamMemberRow) []model.TeamMember {
	out := make([]model.TeamMember, len(rows))
	for i := range rows {
		out[i] = *toModelTeamMember(&rows[i])
	}
	return out
}

func fromModelTeamMember(m *model.TeamMember) *teamMemberRow {
	if m == nil {
		return nil
	}
	return &teamMemberRow{
		ID:        m.ID,
		TeamID:    m.TeamID,
		UserID:    m.UserID,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
	}
}

func toModelWorkflow(row *workflowRow) *model.Workflow {
	if row == nil {
		return nil
	}
	return &model.Workflow{
		ID:          row.ID,
		WorkflowID:  row.WorkflowID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		Description: row.Description,
		Definition:  row.Definition,
		Status:      row.Status,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toModelWorkflows(rows []workflowRow) []model.Workflow {
	out := make([]model.Workflow, len(rows))
	for i := range rows {
		out[i] = *toModelWorkflow(&rows[i])
	}
	return out
}

func fromModelWorkflow(m *model.Workflow) *workflowRow {
	if m == nil {
		return nil
	}
	return &workflowRow{
		ID:          m.ID,
		WorkflowID:  m.WorkflowID,
		TeamID:      m.TeamID,
		Name:        m.Name,
		Description: m.Description,
		Definition:  m.Definition,
		Status:      m.Status,
		CreatedBy:   m.CreatedBy,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func toModelWorkflowRun(row *workflowRunRow) *model.WorkflowRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowRun{
		ID:             row.ID,
		WorkflowRunID:  row.WorkflowRunID,
		WorkflowID:     row.WorkflowID,
		IssueID:        row.IssueID,
		ConversationID: row.ConversationID,
		Status:         row.Status,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
		StartedAt:      row.StartedAt,
		EndedAt:        row.EndedAt,
		ErrorMessage:   row.ErrorMessage,
	}
}

func toModelWorkflowRuns(rows []workflowRunRow) []model.WorkflowRun {
	out := make([]model.WorkflowRun, len(rows))
	for i := range rows {
		out[i] = *toModelWorkflowRun(&rows[i])
	}
	return out
}

func fromModelWorkflowRun(m *model.WorkflowRun) *workflowRunRow {
	if m == nil {
		return nil
	}
	return &workflowRunRow{
		ID:             m.ID,
		WorkflowRunID:  m.WorkflowRunID,
		WorkflowID:     m.WorkflowID,
		IssueID:        m.IssueID,
		ConversationID: m.ConversationID,
		Status:         m.Status,
		CreatedBy:      m.CreatedBy,
		CreatedAt:      m.CreatedAt,
		StartedAt:      m.StartedAt,
		EndedAt:        m.EndedAt,
		ErrorMessage:   m.ErrorMessage,
	}
}

func toModelWorkflowStepRun(row *workflowStepRunRow) *model.WorkflowStepRun {
	if row == nil {
		return nil
	}
	return &model.WorkflowStepRun{
		ID:            row.ID,
		StepRunID:     row.StepRunID,
		WorkflowRunID: row.WorkflowRunID,
		StepID:        row.StepID,
		StepIndex:     row.StepIndex,
		StepType:      row.StepType,
		TargetAgentID: row.TargetAgentID,
		Prompt:        row.Prompt,
		Status:        row.Status,
		TaskID:        row.TaskID,
		TaskRunID:     row.TaskRunID,
		OutputSummary: row.OutputSummary,
		ErrorMessage:  row.ErrorMessage,
		CreatedAt:     row.CreatedAt,
		StartedAt:     row.StartedAt,
		EndedAt:       row.EndedAt,
	}
}

func toModelWorkflowStepRuns(rows []workflowStepRunRow) []model.WorkflowStepRun {
	out := make([]model.WorkflowStepRun, len(rows))
	for i := range rows {
		out[i] = *toModelWorkflowStepRun(&rows[i])
	}
	return out
}

func fromModelWorkflowStepRun(m *model.WorkflowStepRun) *workflowStepRunRow {
	if m == nil {
		return nil
	}
	return &workflowStepRunRow{
		ID:            m.ID,
		StepRunID:     m.StepRunID,
		WorkflowRunID: m.WorkflowRunID,
		StepID:        m.StepID,
		StepIndex:     m.StepIndex,
		StepType:      m.StepType,
		TargetAgentID: m.TargetAgentID,
		Prompt:        m.Prompt,
		Status:        m.Status,
		TaskID:        m.TaskID,
		TaskRunID:     m.TaskRunID,
		OutputSummary: m.OutputSummary,
		ErrorMessage:  m.ErrorMessage,
		CreatedAt:     m.CreatedAt,
		StartedAt:     m.StartedAt,
		EndedAt:       m.EndedAt,
	}
}

func toModelIssue(row *issueRow) *model.Issue {
	if row == nil {
		return nil
	}
	return &model.Issue{
		ID:           row.ID,
		IssueID:      row.IssueID,
		UserID:       row.UserID,
		TeamID:       row.TeamID,
		Title:        row.Title,
		Description:  row.Description,
		Status:       row.Status,
		AssigneeKind: row.AssigneeKind,
		AssigneeID:   row.AssigneeID,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func toModelIssues(rows []issueRow) []model.Issue {
	out := make([]model.Issue, len(rows))
	for i := range rows {
		out[i] = *toModelIssue(&rows[i])
	}
	return out
}

func fromModelIssue(m *model.Issue) *issueRow {
	if m == nil {
		return nil
	}
	return &issueRow{
		ID:           m.ID,
		IssueID:      m.IssueID,
		UserID:       m.UserID,
		TeamID:       m.TeamID,
		Title:        m.Title,
		Description:  m.Description,
		Status:       m.Status,
		AssigneeKind: m.AssigneeKind,
		AssigneeID:   m.AssigneeID,
		CreatedBy:    m.CreatedBy,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toModelTask(row *taskRow) *model.Task {
	if row == nil {
		return nil
	}
	return &model.Task{
		ID:                    row.ID,
		TaskID:                row.TaskID,
		ConversationID:        row.ConversationID,
		TeamID:                row.TeamID,
		IssueID:               row.IssueID,
		Status:                row.Status,
		Input:                 row.Input,
		Title:                 row.Title,
		TitlePromptTokens:     row.TitlePromptTokens,
		TitleCompletionTokens: row.TitleCompletionTokens,
		Output:                row.Output,
		CreatedBy:             row.CreatedBy,
		CreatedAt:             row.CreatedAt,
		StartedAt:             row.StartedAt,
		EndedAt:               row.EndedAt,
		ErrorMessage:          row.ErrorMessage,
		SessionID:             row.SessionID,
		LastRunID:             row.LastRunID,
		AgentID:               row.AgentID,
	}
}

func toModelTasks(rows []taskRow) []model.Task {
	out := make([]model.Task, len(rows))
	for i := range rows {
		out[i] = *toModelTask(&rows[i])
	}
	return out
}

func fromModelTask(m *model.Task) *taskRow {
	if m == nil {
		return nil
	}
	return &taskRow{
		ID:                    m.ID,
		TaskID:                m.TaskID,
		ConversationID:        m.ConversationID,
		TeamID:                m.TeamID,
		IssueID:               m.IssueID,
		Status:                m.Status,
		Input:                 m.Input,
		Title:                 m.Title,
		TitlePromptTokens:     m.TitlePromptTokens,
		TitleCompletionTokens: m.TitleCompletionTokens,
		Output:                m.Output,
		CreatedBy:             m.CreatedBy,
		CreatedAt:             m.CreatedAt,
		StartedAt:             m.StartedAt,
		EndedAt:               m.EndedAt,
		ErrorMessage:          m.ErrorMessage,
		SessionID:             m.SessionID,
		LastRunID:             m.LastRunID,
		AgentID:               m.AgentID,
	}
}

func toModelTaskRun(row *taskRunRow) *model.TaskRun {
	if row == nil {
		return nil
	}
	return &model.TaskRun{
		ID:               row.ID,
		TaskRunID:        row.TaskRunID,
		TaskID:           row.TaskID,
		Input:            row.Input,
		CreatedBy:        row.CreatedBy,
		CreatedByType:    row.CreatedByType,
		TriggerSource:    row.TriggerSource,
		Status:           row.Status,
		Output:           row.Output,
		ErrorMessage:     row.ErrorMessage,
		StartedAt:        row.StartedAt,
		EndedAt:          row.EndedAt,
		SessionID:        row.SessionID,
		WorkerType:       row.WorkerType,
		K8sJobName:       row.K8sJobName,
		K8sJobCreatedAt:  row.K8sJobCreatedAt,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		CreatedAt:        row.CreatedAt,
	}
}

func toModelTaskRuns(rows []taskRunRow) []model.TaskRun {
	out := make([]model.TaskRun, len(rows))
	for i := range rows {
		out[i] = *toModelTaskRun(&rows[i])
	}
	return out
}

func fromModelTaskRun(m *model.TaskRun) *taskRunRow {
	if m == nil {
		return nil
	}
	return &taskRunRow{
		ID:               m.ID,
		TaskRunID:        m.TaskRunID,
		TaskID:           m.TaskID,
		Input:            m.Input,
		CreatedBy:        m.CreatedBy,
		CreatedByType:    m.CreatedByType,
		TriggerSource:    m.TriggerSource,
		Status:           m.Status,
		Output:           m.Output,
		ErrorMessage:     m.ErrorMessage,
		StartedAt:        m.StartedAt,
		EndedAt:          m.EndedAt,
		SessionID:        m.SessionID,
		WorkerType:       m.WorkerType,
		K8sJobName:       m.K8sJobName,
		K8sJobCreatedAt:  m.K8sJobCreatedAt,
		PromptTokens:     m.PromptTokens,
		CompletionTokens: m.CompletionTokens,
		CreatedAt:        m.CreatedAt,
	}
}

func toModelTaskRunArtifact(row *taskRunArtifactRow) *model.TaskRunArtifact {
	if row == nil {
		return nil
	}
	return &model.TaskRunArtifact{
		ID:           row.ID,
		TaskRunID:    row.TaskRunID,
		RelativePath: row.RelativePath,
	}
}

func toModelTaskRunArtifacts(rows []taskRunArtifactRow) []model.TaskRunArtifact {
	out := make([]model.TaskRunArtifact, len(rows))
	for i := range rows {
		out[i] = *toModelTaskRunArtifact(&rows[i])
	}
	return out
}

func fromModelTaskRunArtifact(m *model.TaskRunArtifact) *taskRunArtifactRow {
	if m == nil {
		return nil
	}
	return &taskRunArtifactRow{
		ID:           m.ID,
		TaskRunID:    m.TaskRunID,
		RelativePath: m.RelativePath,
	}
}

func toModelAgent(row *agentRow) *model.AgentDefinition {
	if row == nil {
		return nil
	}
	return &model.AgentDefinition{
		ID:           row.ID,
		AgentID:      row.AgentID,
		UserID:       row.UserID,
		TeamID:       row.TeamID,
		Name:         row.Name,
		Description:  row.Description,
		Instructions: row.Instructions,
		CreatedAt:    row.CreatedAt,
	}
}

func toModelAgents(rows []agentRow) []model.AgentDefinition {
	out := make([]model.AgentDefinition, len(rows))
	for i := range rows {
		out[i] = *toModelAgent(&rows[i])
	}
	return out
}

func fromModelAgent(m *model.AgentDefinition) *agentRow {
	if m == nil {
		return nil
	}
	return &agentRow{
		ID:           m.ID,
		AgentID:      m.AgentID,
		UserID:       m.UserID,
		TeamID:       m.TeamID,
		Name:         m.Name,
		Description:  m.Description,
		Instructions: m.Instructions,
		CreatedAt:    m.CreatedAt,
	}
}

func toModelQuotaTier(row *quotaTierRow) *model.QuotaTier {
	if row == nil {
		return nil
	}
	return &model.QuotaTier{
		TierName:           row.TierName,
		MaxRunsPerPeriod:   row.MaxRunsPerPeriod,
		MaxTokensPerPeriod: row.MaxTokensPerPeriod,
		PeriodDays:         row.PeriodDays,
	}
}

func toModelConversation(row *conversationRow) *model.Conversation {
	if row == nil {
		return nil
	}
	return &model.Conversation{
		ID:             row.ID,
		ConversationID: row.ConversationID,
		UserID:         row.UserID,
		TeamID:         row.TeamID,
		Channel:        row.Channel,
		Title:          row.Title,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
	}
}

func toModelConversations(rows []conversationRow) []model.Conversation {
	out := make([]model.Conversation, len(rows))
	for i := range rows {
		out[i] = *toModelConversation(&rows[i])
	}
	return out
}

func fromModelConversation(m *model.Conversation) *conversationRow {
	if m == nil {
		return nil
	}
	return &conversationRow{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		UserID:         m.UserID,
		TeamID:         m.TeamID,
		Channel:        m.Channel,
		Title:          m.Title,
		CreatedBy:      m.CreatedBy,
		CreatedAt:      m.CreatedAt,
	}
}

func toModelConversationMessage(row *conversationMessageRow) *model.ConversationMessage {
	if row == nil {
		return nil
	}
	return &model.ConversationMessage{
		ID:                    row.ID,
		ConversationMessageID: row.ConversationMessageID,
		ConversationID:        row.ConversationID,
		Role:                  row.Role,
		Content:               row.Content,
		Channel:               row.Channel,
		ToolCallID:            row.ToolCallID,
		ToolCallsJSON:         row.ToolCallsJSON,
		CreatedAt:             row.CreatedAt,
	}
}

func toModelConversationMessages(rows []conversationMessageRow) []model.ConversationMessage {
	out := make([]model.ConversationMessage, len(rows))
	for i := range rows {
		out[i] = *toModelConversationMessage(&rows[i])
	}
	return out
}

func fromModelConversationMessage(m *model.ConversationMessage) *conversationMessageRow {
	if m == nil {
		return nil
	}
	return &conversationMessageRow{
		ID:                    m.ID,
		ConversationMessageID: m.ConversationMessageID,
		ConversationID:        m.ConversationID,
		Role:                  m.Role,
		Content:               m.Content,
		Channel:               m.Channel,
		ToolCallID:            m.ToolCallID,
		ToolCallsJSON:         m.ToolCallsJSON,
		CreatedAt:             m.CreatedAt,
	}
}

func toModelUserWebhookKey(row *userWebhookKeyRow) *model.UserWebhookKey {
	if row == nil {
		return nil
	}
	return &model.UserWebhookKey{
		ID:        row.ID,
		KeyID:     row.KeyID,
		UserID:    row.UserID,
		KeyHash:   row.KeyHash,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
	}
}

func toModelUserWebhookKeys(rows []userWebhookKeyRow) []model.UserWebhookKey {
	out := make([]model.UserWebhookKey, len(rows))
	for i := range rows {
		out[i] = *toModelUserWebhookKey(&rows[i])
	}
	return out
}

func fromModelUserWebhookKey(m *model.UserWebhookKey) *userWebhookKeyRow {
	if m == nil {
		return nil
	}
	return &userWebhookKeyRow{
		ID:        m.ID,
		KeyID:     m.KeyID,
		UserID:    m.UserID,
		KeyHash:   m.KeyHash,
		Name:      m.Name,
		CreatedAt: m.CreatedAt,
	}
}
