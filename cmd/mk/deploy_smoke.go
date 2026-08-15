package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	composeFile      = "deployment/compose/compose.yaml"
	composeSmokeFile = "deployment/compose/compose.smoke.yaml"
	smokeEmail       = "deployment-smoke@buildmax.local"
	smokeReply       = "deployment smoke ok"
)

var loginCodePattern = regexp.MustCompile(`bmxlogin_[a-f0-9]+`)

type smokeTarget struct {
	apiBase              string
	portalURL            string
	portalRuntimeAPIBase string
	admin                func(args ...string) (string, error)
}

func cmdCompose(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ./make compose <up|smoke|logs|down>")
	}
	switch args[0] {
	case "up":
		if err := ensureComposeEnv(); err != nil {
			return err
		}
		return runCmd("docker", "compose", "-f", composeFile, "up", "-d", "--build", "--wait")
	case "smoke":
		if err := ensureComposeEnv(); err != nil {
			return err
		}
		files := composeSmokeArgs()
		if err := runCmd("docker", append(files, "up", "-d", "--build", "--wait")...); err != nil {
			return err
		}
		target := smokeTarget{
			apiBase:              "http://localhost:" + envOr("BUILDMAX_SERVER_PORT", "5678"),
			portalURL:            "http://localhost:" + envOr("BUILDMAX_PORTAL_PORT", "8080"),
			portalRuntimeAPIBase: "http://localhost:" + envOr("BUILDMAX_SERVER_PORT", "5678"),
			admin: func(args ...string) (string, error) {
				cmdArgs := append(composeSmokeArgs(), "exec", "-T", "server", "buildmax-server")
				return captureCombined("docker", append(cmdArgs, args...)...)
			},
		}
		if err := runDeploymentSmoke(context.Background(), target); err != nil {
			return err
		}
		printSmokeLogin(target)
		return nil
	case "logs":
		return runCmd("docker", append(composeSmokeArgs(), "logs", "--tail", "200")...)
	case "down":
		return runCmd("docker", append(composeSmokeArgs(), "down")...)
	default:
		return fmt.Errorf("unknown compose command %q (want up, smoke, logs, or down)", args[0])
	}
}

func composeSmokeArgs() []string {
	return []string{"compose", "-f", composeFile, "-f", composeSmokeFile}
}

func ensureComposeEnv() error {
	envPath := filepath.Join("deployment", "compose", ".env")
	if exists(envPath) {
		return nil
	}
	return runCmd(filepath.Join("deployment", "compose", "generate-env.sh"))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func captureCombined(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
	}
	return text, nil
}

func runDeploymentSmoke(ctx context.Context, target smokeTarget) error {
	client := &http.Client{Timeout: 10 * time.Second}
	if err := waitForHTTP(ctx, client, target.apiBase+"/healthz", 90*time.Second); err != nil {
		return err
	}
	if err := expectHTTPStatus(ctx, client, target.portalURL, http.StatusOK); err != nil {
		return fmt.Errorf("portal: %w", err)
	}
	portalConfig, err := requestText(ctx, client, http.MethodGet, strings.TrimRight(target.portalURL, "/")+"/config.js", "", nil, http.StatusOK)
	if err != nil {
		return fmt.Errorf("portal runtime config: %w", err)
	}
	wantAPIBase := fmt.Sprintf("apiBase: %q", target.portalRuntimeAPIBase)
	if !strings.Contains(portalConfig, wantAPIBase) {
		return fmt.Errorf("portal runtime config does not contain %q: %s", wantAPIBase, strings.TrimSpace(portalConfig))
	}

	if output, err := target.admin("user", "create", smokeEmail); err != nil && !strings.Contains(output, "already has an account") {
		return fmt.Errorf("create smoke account: %w", err)
	}
	codeOutput, err := target.admin("user", "login-code", smokeEmail)
	if err != nil {
		return fmt.Errorf("issue smoke login code: %w", err)
	}
	code := loginCodePattern.FindString(codeOutput)
	if code == "" {
		return errors.New("login code command returned no bmxlogin_ code")
	}

	var login struct {
		Token string `json:"token"`
	}
	if err := requestJSON(ctx, client, http.MethodPost, target.apiBase+"/api/login", "", map[string]string{
		"email": smokeEmail, "otp": code, "platform": "deployment-smoke",
	}, &login, http.StatusOK); err != nil {
		return err
	}
	if login.Token == "" {
		return errors.New("login response contained no token")
	}

	var teams []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := requestJSON(ctx, client, http.MethodGet, target.apiBase+"/api/teams", login.Token, nil, &teams, http.StatusOK); err != nil {
		return err
	}
	if len(teams) == 0 || teams[0].ID == "" {
		return errors.New("smoke account has no personal team")
	}
	teamID := teams[0].ID

	if err := uploadSmokeFile(ctx, client, target.apiBase, teamID, login.Token); err != nil {
		return err
	}
	fileURL := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/files/deployment-smoke.txt"
	content, err := requestText(ctx, client, http.MethodGet, fileURL, login.Token, nil, http.StatusOK)
	if err != nil {
		return err
	}
	if content != smokeReply {
		return fmt.Errorf("uploaded file content = %q, want %q", content, smokeReply)
	}

	var conversation struct {
		ID string `json:"conversation_id"`
	}
	conversationsURL := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/conversations"
	if err := requestJSON(ctx, client, http.MethodPost, conversationsURL, login.Token, map[string]string{"channel": "portal"}, &conversation, http.StatusCreated); err != nil {
		return err
	}
	var task struct {
		ID string `json:"id"`
	}
	tasksURL := conversationsURL + "/" + url.PathEscape(conversation.ID) + "/tasks"
	if err := requestJSON(ctx, client, http.MethodPost, tasksURL, login.Token, map[string]string{"input": "Reply with exactly deployment smoke ok."}, &task, http.StatusCreated); err != nil {
		return err
	}

	var completed struct {
		Status       string  `json:"status"`
		Output       *string `json:"output"`
		ErrorMessage *string `json:"error_message"`
	}
	taskURL := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/tasks/" + url.PathEscape(task.ID)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if err := requestJSON(ctx, client, http.MethodGet, taskURL, login.Token, nil, &completed, http.StatusOK); err != nil {
			return err
		}
		if completed.Status == "SUCCEEDED" {
			break
		}
		if completed.Status == "FAILED" {
			return fmt.Errorf("task run failed: %s", stringValue(completed.ErrorMessage))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("task did not finish within two minutes (status %s)", completed.Status)
		}
		time.Sleep(time.Second)
	}
	if completed.Output == nil || strings.TrimSpace(*completed.Output) != smokeReply {
		return fmt.Errorf("task output = %q, want %q", stringValue(completed.Output), smokeReply)
	}

	var artifacts []struct {
		TaskRunID string `json:"task_run_id"`
	}
	if err := requestJSON(ctx, client, http.MethodGet, taskURL+"/artifacts", login.Token, nil, &artifacts, http.StatusOK); err != nil {
		return err
	}
	if len(artifacts) == 0 || artifacts[0].TaskRunID == "" {
		return errors.New("successful task has no artifact")
	}
	artifactURL := target.apiBase + "/api/teams/" + url.PathEscape(teamID) + "/task-runs/" + url.PathEscape(artifacts[0].TaskRunID) + "/artifacts/content"
	artifact, err := requestText(ctx, client, http.MethodGet, artifactURL, login.Token, nil, http.StatusOK)
	if err != nil {
		return err
	}
	if strings.TrimSpace(artifact) != smokeReply {
		return fmt.Errorf("artifact content = %q, want %q", strings.TrimSpace(artifact), smokeReply)
	}

	fmt.Printf("Deployment smoke passed: portal, auth, team, storage, scheduler, worker, and artifact (%s)\n", target.portalURL)
	return nil
}

func printSmokeLogin(target smokeTarget) {
	output, err := target.admin("user", "login-code", smokeEmail)
	if err != nil {
		fmt.Printf("Smoke passed, but issuing an interactive login code failed: %v\n", err)
		return
	}
	fmt.Printf("Open %s and sign in as %s with this single-use code:\n%s\n", target.portalURL, smokeEmail, output)
}

func waitForHTTP(ctx context.Context, client *http.Client, endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := expectHTTPStatus(ctx, client, endpoint, http.StatusOK); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become ready within %s", endpoint, timeout)
		}
		time.Sleep(time.Second)
	}
}

func expectHTTPStatus(ctx context.Context, client *http.Client, endpoint string, want int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		return fmt.Errorf("GET %s returned %s", endpoint, resp.Status)
	}
	return nil
}

func requestJSON(ctx context.Context, client *http.Client, method, endpoint, token string, body, out any, want int) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	response, err := request(ctx, client, method, endpoint, token, "application/json", reader, want)
	if err != nil {
		return err
	}
	defer response.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(response).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, endpoint, err)
	}
	return nil
}

func requestText(ctx context.Context, client *http.Client, method, endpoint, token string, body io.Reader, want int) (string, error) {
	response, err := request(ctx, client, method, endpoint, token, "", body, want)
	if err != nil {
		return "", err
	}
	defer response.Close()
	data, err := io.ReadAll(response)
	return string(data), err
}

func request(ctx context.Context, client *http.Client, method, endpoint, token, contentType string, body io.Reader, want int) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != want {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s %s returned %s: %s", method, endpoint, resp.Status, strings.TrimSpace(string(data)))
	}
	return resp.Body, nil
}

func uploadSmokeFile(ctx context.Context, client *http.Client, apiBase, teamID, token string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "deployment-smoke.txt")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, smokeReply); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	endpoint := apiBase + "/api/teams/" + url.PathEscape(teamID) + "/upload"
	response, err := request(ctx, client, http.MethodPost, endpoint, token, writer.FormDataContentType(), &body, http.StatusOK)
	if err != nil {
		return err
	}
	return response.Close()
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
