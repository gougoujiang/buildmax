package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const provenanceSecret = "test-run-provenance-secret"

type provenanceFixture struct {
	handler  *Handler
	mux      *http.ServeMux
	messages *mock.MockConversationMessageStore
}

func newProvenanceFixture(t *testing.T, run model.TaskRun, task model.Task) provenanceFixture {
	t.Helper()
	messages := &mock.MockConversationMessageStore{}
	h := New(Config{
		JWTSecret: provenanceSecret,
		Teams: &mock.MockTeamStore{
			Teams:   []model.Team{{ID: "tm_1", Name: "My Space", PersonalForUserID: util.Ptr("u1"), CreatedBy: "u1"}},
			Members: []model.TeamMember{{TeamID: "tm_1", UserID: "u1", Role: model.TeamRoleOwner}},
		},
		Conversations: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ID: "conv1", UserID: "u1", TeamID: "tm_1", Channel: "portal", CreatedBy: "u1"}},
		},
		Tasks:    &mock.MockTaskStore{List: []model.Task{task}},
		TaskRuns: &mock.MockTaskRunStore{Runs: []model.TaskRun{run}, TaskList: []model.Task{task}},
		Messages: messages,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return provenanceFixture{handler: h, mux: mux, messages: messages}
}

func (f provenanceFixture) get(t *testing.T, taskRunID string) (int, RunProvenanceResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/teams/tm_1/task-runs/"+taskRunID, nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", provenanceSecret))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	var out RunProvenanceResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return rec.Code, out
}

func provenanceTask() model.Task {
	return model.Task{ID: "tk_1", ConversationID: "conv1", TeamID: "tm_1", Status: "SUCCEEDED", Input: "x", CreatedBy: "u1"}
}

// The route exists so the instruction a worker was given can be read next to
// what the person actually asked for. Returning one without the other would
// make it pointless.
func TestRunProvenanceQuotesTheMessageBehindTheRun(t *testing.T) {
	run := model.TaskRun{
		ID: "tr_1", TaskID: "tk_1", Input: "investigate the flaky test",
		Status: "SUCCEEDED", CreatedBy: "u1", CreatedByType: model.RunCreatedByTypeUser,
		TriggerSource: model.RunTriggerSourcePortalConversation, CreatedAt: 1000,
	}
	f := newProvenanceFixture(t, run, provenanceTask())
	asked, err := f.messages.AppendMessage(t.Context(), model.AppendMessageInput{
		ConversationID: "conv1",
		Role:           "user",
		Content:        "look into the flaky test, but leave the CI config alone",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.handler.cfg.TaskRuns.(*mock.MockTaskRunStore).Runs[0].SourceMessageID = &asked.ID

	code, out := f.get(t, "tr_1")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if out.SourceMessage == nil {
		t.Fatal("no source message returned")
	}
	if out.SourceMessage.ID != asked.ID || out.SourceMessage.Content != asked.Content {
		t.Errorf("source message = %+v, want %s / %q", out.SourceMessage, asked.ID, asked.Content)
	}
	if out.SourceMessage.Truncated {
		t.Error("a short message should not be marked truncated")
	}
	if out.Input != run.Input {
		t.Errorf("input = %q, want the run's own instruction", out.Input)
	}
	if out.TriggerSource != model.RunTriggerSourcePortalConversation {
		t.Errorf("trigger_source = %q", out.TriggerSource)
	}
}

// A long message is quoted for comparison, not served whole: the conversation
// route is where the transcript lives.
func TestRunProvenanceTruncatesALongMessage(t *testing.T) {
	run := model.TaskRun{ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED", CreatedAt: 1000}
	f := newProvenanceFixture(t, run, provenanceTask())
	asked, err := f.messages.AppendMessage(t.Context(), model.AppendMessageInput{
		ConversationID: "conv1",
		Role:           "user",
		Content:        strings.Repeat("a", sourceMessageMaxLen+50),
	})
	if err != nil {
		t.Fatal(err)
	}
	f.handler.cfg.TaskRuns.(*mock.MockTaskRunStore).Runs[0].SourceMessageID = &asked.ID

	_, out := f.get(t, "tr_1")
	if out.SourceMessage == nil || !out.SourceMessage.Truncated {
		t.Fatalf("source message = %+v, want a truncated quote", out.SourceMessage)
	}
	if len([]rune(out.SourceMessage.Content)) > sourceMessageMaxLen+1 {
		t.Errorf("quote is %d runes, want at most %d plus the ellipsis", len([]rune(out.SourceMessage.Content)), sourceMessageMaxLen)
	}
}

// A run with no message behind it is normal — a workflow step, an issue agent
// run, a retry. The rest of the provenance is still true and still answered.
func TestRunProvenanceWithoutASourceMessage(t *testing.T) {
	run := model.TaskRun{
		ID: "tr_1", TaskID: "tk_1", Input: "step 2", Status: "RUNNING",
		CreatedByType: model.RunCreatedByTypeSystem, TriggerSource: model.RunTriggerSourceWorkflowStep, CreatedAt: 1000,
	}
	f := newProvenanceFixture(t, run, provenanceTask())

	code, out := f.get(t, "tr_1")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if out.SourceMessage != nil {
		t.Errorf("source message = %+v, want none", out.SourceMessage)
	}
	if out.TriggerSource != model.RunTriggerSourceWorkflowStep {
		t.Errorf("trigger_source = %q, want the workflow step", out.TriggerSource)
	}
}

// A handle pointing outside the run's own conversation quotes nothing. It is
// how a stale or wrong reference stops being a way to read someone else's text.
func TestRunProvenanceIgnoresAMessageFromAnotherConversation(t *testing.T) {
	run := model.TaskRun{ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED", CreatedAt: 1000}
	f := newProvenanceFixture(t, run, provenanceTask())
	elsewhere, err := f.messages.AppendMessage(t.Context(), model.AppendMessageInput{
		ConversationID: "conv-other",
		Role:           "user",
		Content:        "something said in another conversation",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.handler.cfg.TaskRuns.(*mock.MockTaskRunStore).Runs[0].SourceMessageID = &elsewhere.ID

	_, out := f.get(t, "tr_1")
	if out.SourceMessage != nil {
		t.Errorf("source message = %+v, want none", out.SourceMessage)
	}
}

// The run belongs to a team, and a stranger to that team cannot read where it
// came from any more than what it produced.
func TestRunProvenanceRefusesAnotherTeam(t *testing.T) {
	run := model.TaskRun{ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED", CreatedAt: 1000}
	f := newProvenanceFixture(t, run, provenanceTask())

	req := httptest.NewRequest(http.MethodGet, "/api/teams/tm_other/task-runs/tr_1", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u1", provenanceSecret))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200, want a refusal; body = %s", rec.Body.String())
	}
}

// An agent's instructions are resolved when its worker asks for the run, so two
// runs of one task can execute different text. The run names the revision it
// was handed, and the response says when the definition has moved on since.
func TestRunProvenanceNamesTheAgentRevisionThatRan(t *testing.T) {
	revision := 2
	run := model.TaskRun{
		ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED",
		AgentRevision: &revision, CreatedAt: 1000,
	}
	task := provenanceTask()
	task.AgentID = util.Ptr("ag_1")
	f := newProvenanceFixture(t, run, task)
	f.handler.cfg.Agents = &mock.MockAgentStore{Agents: []model.Agent{
		{ID: "ag_1", TeamID: "tm_1", Name: "Reviewer", Revision: 5},
	}}

	_, out := f.get(t, "tr_1")
	if out.Agent == nil {
		t.Fatal("no agent returned")
	}
	if out.Agent.Revision != 2 {
		t.Errorf("revision = %d, want the one the run was handed", out.Agent.Revision)
	}
	if out.Agent.CurrentRevision != 5 {
		t.Errorf("current_revision = %d, want what the definition says now", out.Agent.CurrentRevision)
	}
	if out.Agent.Name != "Reviewer" {
		t.Errorf("name = %q", out.Agent.Name)
	}
}

// A deleted agent is still named. A run that already executed under it does not
// stop having done so, and hiding the definition is the opposite of provenance.
func TestRunProvenanceNamesADeletedAgent(t *testing.T) {
	revision := 1
	run := model.TaskRun{ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED", AgentRevision: &revision, CreatedAt: 1000}
	task := provenanceTask()
	task.AgentID = util.Ptr("ag_1")
	f := newProvenanceFixture(t, run, task)
	f.handler.cfg.Agents = &mock.MockAgentStore{Agents: []model.Agent{
		{ID: "ag_1", TeamID: "tm_1", Name: "Retired", Revision: 1, DeletedAt: util.Ptr(int64(9))},
	}}

	_, out := f.get(t, "tr_1")
	if out.Agent == nil || !out.Agent.Deleted || out.Agent.Name != "Retired" {
		t.Fatalf("agent = %+v, want the deleted definition named", out.Agent)
	}
}

// A task with no agent has no agent block at all, rather than an empty one that
// reads as an agent nobody can identify.
func TestRunProvenanceOmitsTheAgentWhenThereIsNone(t *testing.T) {
	run := model.TaskRun{ID: "tr_1", TaskID: "tk_1", Input: "do it", Status: "SUCCEEDED", CreatedAt: 1000}
	f := newProvenanceFixture(t, run, provenanceTask())

	_, out := f.get(t, "tr_1")
	if out.Agent != nil {
		t.Errorf("agent = %+v, want none", out.Agent)
	}
}
