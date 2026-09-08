package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestFixturePaginationExhaustsServerPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if r.URL.Query().Get("limit") != "100" {
			t.Error("missing page limit")
		}
		// A server may cap pages below the requested limit.
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": []fxIssue{{ID: strconv.Itoa(offset)}}, "total": 3})
	}))
	defer server.Close()
	got, err := fixturePage[fxIssue](context.Background(), server.Client(), server.URL, "token", "issues")
	if err != nil || len(got) != 3 || requests != 3 {
		t.Fatalf("page = %v, requests = %d, err = %v", got, requests, err)
	}
}

func TestFixturePaginationRejectsTruncatedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"issues":[],"total":1}`) }))
	defer server.Close()
	if _, err := fixturePage[fxIssue](context.Background(), server.Client(), server.URL, "token", "issues"); err == nil {
		t.Fatal("accepted an incomplete listing")
	}
}

func TestFixtureCommentsResumePartialSeed(t *testing.T) {
	bodies := []string{"Unrelated user comment", "First fixture comment"}
	failed := false
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			comments := []map[string]string{}
			for _, body := range bodies {
				comments = append(comments, map[string]string{"body": body})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": comments, "total": len(comments)})
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Body == "Third fixture comment" && !failed {
			failed = true
			http.Error(w, "interrupted", 500)
			return
		}
		bodies = append(bodies, req.Body)
		posts++
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	target := smokeTarget{apiBase: server.URL}
	seed := func() error {
		return ensureComments(context.Background(), server.Client(), target, "space", "token", "issue", []string{"First fixture comment", "Second fixture comment", "Third fixture comment"})
	}
	if seed() == nil {
		t.Fatal("expected interrupted seed")
	}
	for i := 0; i < 2; i++ {
		if err := seed(); err != nil {
			t.Fatal(err)
		}
	}
	if posts != 2 || len(bodies) != 4 {
		t.Fatalf("duplicated or missing comments: %v", bodies)
	}
}

func TestFixtureIssuesLinkNewParentAndPreserveStatus(t *testing.T) {
	issues := []fxIssue{{ID: "existing", Title: "Existing", Status: "done", Version: 7}}
	creates, patches := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"issues": issues, "total": len(issues)})
		case http.MethodPost:
			var issue fxIssue
			_ = json.NewDecoder(r.Body).Decode(&issue)
			creates++
			issue.ID, issue.Status, issue.Version = strconv.Itoa(creates), "todo", 1
			if issue.Title == "Child" && issue.ParentIssueID != "1" {
				t.Errorf("child parent = %q", issue.ParentIssueID)
			}
			issues = append(issues, issue)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(issue)
		case http.MethodPatch:
			patches++
			var req struct {
				Version uint64 `json:"version"`
				Status  string `json:"status"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Version != 1 {
				t.Errorf("version = %d", req.Version)
			}
			for i := range issues {
				if r.URL.Path == "/api/spaces/space/issues/"+issues[i].ID {
					issues[i].Status = req.Status
					issues[i].Version++
					_ = json.NewEncoder(w).Encode(issues[i])
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	specs := []fixtureIssue{{title: "Existing", status: "todo"}, {title: "Parent", status: "in_progress"}, {title: "Child", parentTitle: "Parent", status: "todo"}}
	for i := 0; i < 2; i++ {
		if err := ensureIssues(context.Background(), server.Client(), smokeTarget{apiBase: server.URL}, "space", "token", "test", specs); err != nil {
			t.Fatal(err)
		}
	}
	if creates != 2 || patches != 1 || issues[0].Status != "done" {
		t.Fatalf("creates=%d patches=%d issues=%v", creates, patches, issues)
	}
}

func TestExecutionFixturesRejectModelOverrides(t *testing.T) {
	for _, test := range []struct {
		name         string
		env, envFrom string
		wantError    bool
	}{
		{"reference", `[{"name":"BUILDMAX_CONVERSATION_MODEL_API_KEY","valueFrom":{"secretKeyRef":{"name":"secret","key":"key"}}}]`, `[]`, false},
		{"managed worker", `[{"name":"BUILDMAX_WORKER_LLM_TRANSPORT","value":"buildmax"}]`, `[]`, true},
		{"conversation target", `[{"name":"BUILDMAX_CONVERSATION_MODEL_TARGET","value":"paid"}]`, `[]`, true},
		{"worker URL", `[{"name":"BUILDMAX_WORKER_LLM_API_URL","value":"https://example.invalid"}]`, `[]`, true},
		{"indirect environment", `[]`, `[{"configMapRef":{"name":"override"}}]`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"env":%s,"envFrom":%s}]}}}}`, test.env, test.envFrom)
			err := validateFixtureModelEnv(raw)
			if (err != nil) != test.wantError {
				t.Fatalf("err = %v, wantError = %v", err, test.wantError)
			}
		})
	}
}
