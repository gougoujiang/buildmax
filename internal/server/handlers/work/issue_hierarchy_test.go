package work

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreissue "github.com/gougoujiang/buildmax/internal/core/issue"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
	"github.com/gougoujiang/buildmax/internal/util"
)

const hierarchyTestSecret = "hierarchy-test-secret"

const hierarchyTeam = "tm_hierarchy"

// hierarchyMux wires one parent with two children — one done, one not — plus a
// loose top-level issue, which is enough to exercise every listing mode and the
// progress rollup at once.
func hierarchyMux(t *testing.T) (*http.ServeMux, *mock.MockIssueStore) {
	t.Helper()
	issues := &mock.MockIssueStore{
		Issues: []coreissue.Issue{
			{ID: "i_parent", UserID: "u_owner", TeamID: hierarchyTeam, Title: "Parent", Status: coreissue.StatusInProgress},
			{ID: "i_child_a", UserID: "u_owner", TeamID: hierarchyTeam, Title: "Child A", Status: coreissue.StatusDone, ParentIssueID: util.Ptr("i_parent")},
			{ID: "i_child_b", UserID: "u_owner", TeamID: hierarchyTeam, Title: "Child B", Status: coreissue.StatusTodo, ParentIssueID: util.Ptr("i_parent")},
			{ID: "i_loose", UserID: "u_owner", TeamID: hierarchyTeam, Title: "Loose", Status: coreissue.StatusTodo},
		},
	}
	teams := &mock.MockTeamStore{
		Teams:   []coreteam.Team{{ID: hierarchyTeam, Name: "Hierarchy", CreatedBy: "u_owner"}},
		Members: []coreteam.Member{{TeamID: hierarchyTeam, UserID: "u_owner", Role: coreteam.RoleOwner}},
	}
	h := New(Config{
		JWTSecret:     hierarchyTestSecret,
		Teams:         teams,
		Issues:        issues,
		IssueComments: &mock.MockIssueCommentStore{},
		Workflows:     &mock.MockWorkflowStore{},
		Tasks:         &mock.MockTaskStore{},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, issues
}

func hierarchyRequest(t *testing.T, mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT("u_owner", hierarchyTestSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeIssueList(t *testing.T, rec *httptest.ResponseRecorder) issueListResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var out issueListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out
}

// No parent_id lists every issue, sub-issues included. Callers predating the
// hierarchy must not see the endpoint change under them.
func TestListIssues_DefaultIsUnfiltered(t *testing.T) {
	mux, _ := hierarchyMux(t)
	out := decodeIssueList(t, hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues", ""))
	if out.Total != 4 || len(out.Issues) != 4 {
		t.Fatalf("total = %d, len = %d, want 4 and 4", out.Total, len(out.Issues))
	}
}

func TestListIssues_TopLevelOnly(t *testing.T) {
	mux, _ := hierarchyMux(t)
	out := decodeIssueList(t, hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues?parent_id=none", ""))
	if out.Total != 2 {
		t.Fatalf("total = %d, want 2 top-level issues", out.Total)
	}
	for _, issue := range out.Issues {
		if issue.ParentIssueID != nil {
			t.Fatalf("issue %s has parent %s in a top-level listing", issue.ID, *issue.ParentIssueID)
		}
	}
}

func TestListIssues_ByParent(t *testing.T) {
	mux, _ := hierarchyMux(t)
	out := decodeIssueList(t, hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues?parent_id=i_parent", ""))
	if out.Total != 2 {
		t.Fatalf("total = %d, want 2 children", out.Total)
	}
	for _, issue := range out.Issues {
		if issue.ParentIssueID == nil || *issue.ParentIssueID != "i_parent" {
			t.Fatalf("issue %s is not a child of i_parent", issue.ID)
		}
	}
}

// Progress is derived per response. One child of two is done, so the parent
// reports 1/2 and the childless issues report 0/0.
func TestListIssues_ChildProgress(t *testing.T) {
	mux, _ := hierarchyMux(t)
	out := decodeIssueList(t, hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues", ""))
	byID := map[string]IssueResponse{}
	for _, issue := range out.Issues {
		byID[issue.ID] = issue
	}
	if parent := byID["i_parent"]; parent.ChildCount != 2 || parent.DoneChildCount != 1 {
		t.Fatalf("parent progress = %d/%d, want 1/2", parent.DoneChildCount, parent.ChildCount)
	}
	if loose := byID["i_loose"]; loose.ChildCount != 0 || loose.DoneChildCount != 0 {
		t.Fatalf("childless issue reports %d/%d, want 0/0", loose.DoneChildCount, loose.ChildCount)
	}
}

func TestCreateIssue_WithParent(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodPost, "/api/teams/"+hierarchyTeam+"/issues", `{"title":"Child C","parent_issue_id":"i_parent"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	var out IssueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ParentIssueID == nil || *out.ParentIssueID != "i_parent" {
		t.Fatalf("parent_issue_id = %v, want i_parent", out.ParentIssueID)
	}
}

func TestCreateIssue_RejectedParents(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing parent", `{"title":"X","parent_issue_id":"i_ghost"}`},
		{"grandchild", `{"title":"X","parent_issue_id":"i_child_a"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux, store := hierarchyMux(t)
			before := len(store.Issues)
			rec := hierarchyRequest(t, mux, http.MethodPost, "/api/teams/"+hierarchyTeam+"/issues", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
			}
			if len(store.Issues) != before {
				t.Fatalf("rejected create still wrote a row")
			}
		})
	}
}

func TestPatchIssue_Reparent(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodPatch, "/api/teams/"+hierarchyTeam+"/issues/i_loose", `{"parent_issue_id":"i_parent"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var out IssueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ParentIssueID == nil || *out.ParentIssueID != "i_parent" {
		t.Fatalf("parent_issue_id = %v, want i_parent", out.ParentIssueID)
	}
}

func TestPatchIssue_ClearParent(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodPatch, "/api/teams/"+hierarchyTeam+"/issues/i_child_b", `{"parent_issue_id":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var out IssueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ParentIssueID != nil {
		t.Fatalf("parent_issue_id = %v, want cleared", *out.ParentIssueID)
	}
}

func TestPatchIssue_ParentWithChildrenCannotBeAdopted(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodPatch, "/api/teams/"+hierarchyTeam+"/issues/i_parent", `{"parent_issue_id":"i_loose"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPatchIssue_SelfParent(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodPatch, "/api/teams/"+hierarchyTeam+"/issues/i_loose", `{"parent_issue_id":"i_loose"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// The flow response carries children on a parent and a parent on a child, and
// never both — the hierarchy is two levels deep.
func TestIssueFlow_Relatives(t *testing.T) {
	mux, _ := hierarchyMux(t)
	rec := hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues/i_parent/flow", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("parent flow status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var parentFlow issueFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &parentFlow); err != nil {
		t.Fatalf("decode parent flow: %v", err)
	}
	if parentFlow.Parent != nil {
		t.Fatalf("a top-level issue reported a parent: %+v", parentFlow.Parent)
	}
	if len(parentFlow.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(parentFlow.Children))
	}
	if parentFlow.Issue.ChildCount != 2 || parentFlow.Issue.DoneChildCount != 1 {
		t.Fatalf("flow progress = %d/%d, want 1/2", parentFlow.Issue.DoneChildCount, parentFlow.Issue.ChildCount)
	}

	rec = hierarchyRequest(t, mux, http.MethodGet, "/api/teams/"+hierarchyTeam+"/issues/i_child_a/flow", "")
	var childFlow issueFlowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &childFlow); err != nil {
		t.Fatalf("decode child flow: %v", err)
	}
	if childFlow.Parent == nil || childFlow.Parent.ID != "i_parent" {
		t.Fatalf("child parent = %+v, want i_parent", childFlow.Parent)
	}
	if len(childFlow.Children) != 0 {
		t.Fatalf("a sub-issue reported %d children", len(childFlow.Children))
	}
}
