// Command mock-llm serves the deployment smokes a model that never varies.
//
// It is a packaging of internal/testsupport/mockllm, not a second
// implementation: the CLI suites and the Compose and kind smokes replay the
// same scenario format, so a reply shape only has to be right in one place.
// See docs/design/end-to-end-testing.md §4.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gougoujiang/buildmax/internal/testsupport/mockllm"
)

// defaultReply is what the deployment smoke asserts a task run produced.
const defaultReply = "deployment smoke ok"

// scenarioPath names a scenario file to replay instead of the default. A
// deployment smoke that wants tool calls mounts one and sets this.
const scenarioPath = "BUILDMAX_MOCK_SCENARIO"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		healthcheck()
		return
	}
	scenario, err := scenario()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", mockllm.NewHandler(scenario))
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// scenario loads the mounted script, or answers every call with the smoke's
// one sentence. The default repeats because a deployment smoke asserts on what
// a run produced, not on how many model calls producing it took.
func scenario() (mockllm.Scenario, error) {
	if path := os.Getenv(scenarioPath); path != "" {
		return mockllm.LoadScenario(path)
	}
	return mockllm.Scenario{
		Name:   "deployment smoke",
		Steps:  []mockllm.Step{{Text: defaultReply, Usage: &mockllm.Usage{PromptTokens: 3, CompletionTokens: 3}}},
		Repeat: true,
	}, nil
}

func healthcheck() {
	resp, err := http.Get("http://127.0.0.1:8080/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	_ = resp.Body.Close()
}
