// Package client provides an HTTP client for the BuildMax server API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
)

// LoginUser is the user subset returned in a login response.
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// LoginResponse is the successful result of POST /api/login.
type LoginResponse struct {
	// Token is AccessToken under the name it had before a login returned two
	// credentials. A server older than that split sends only this one.
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	// RefreshToken is empty when the server keeps no store for it, which means
	// the login ends when the access token does.
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"`
	User         LoginUser `json:"user"`
}

// Access returns the access token under whichever name the server used.
func (r *LoginResponse) Access() string {
	if r.AccessToken != "" {
		return r.AccessToken
	}
	return r.Token
}

// RefreshResponse is the successful result of POST /api/token/refresh.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ErrRefreshRejected means the server refused the refresh token: it is spent,
// revoked, expired, or was replayed. The session is over and only a new login
// will produce another.
var ErrRefreshRejected = errors.New("refresh token rejected")

// Client is a stateless HTTP client for the BuildMax server API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a Client for the given server base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

// RequestOTP calls POST /api/otp/request. intent is "login" or "signup".
func (c *Client) RequestOTP(ctx context.Context, email, intent string) error {
	body, _ := json.Marshal(map[string]string{
		"email":  email,
		"intent": intent,
	})
	url := c.BaseURL + "/api/otp/request"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return parseErrorResponse(resp)
}

// Login calls POST /api/login with a single-use login code — the recovery
// path, used to claim a new account or replace a forgotten password.
// platform identifies the calling client ("cli", "desktop", "portal").
func (c *Client) Login(ctx context.Context, email, otp, platform string) (*LoginResponse, error) {
	return c.login(ctx, map[string]string{
		"email":    email,
		"otp":      otp,
		"platform": platform,
	})
}

// LoginWithPassword calls POST /api/login with a password, the everyday way in.
func (c *Client) LoginWithPassword(ctx context.Context, email, password, platform string) (*LoginResponse, error) {
	return c.login(ctx, map[string]string{
		"email":    email,
		"password": password,
		"platform": platform,
	})
}

func (c *Client) login(ctx context.Context, payload map[string]string) (*LoginResponse, error) {
	body, _ := json.Marshal(payload)
	url := c.BaseURL + "/api/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}
	var lr LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &lr, nil
}

// Refresh calls POST /api/token/refresh, exchanging a refresh token for a new
// pair.
//
// A rejected token returns ErrRefreshRejected, which the caller must be able to
// tell apart from the server being unreachable: one means sign in again, the
// other means try later.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*RefreshResponse, error) {
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	url := c.BaseURL + "/api/token/refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrRefreshRejected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}
	var rr RefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if rr.AccessToken == "" {
		return nil, fmt.Errorf("refresh returned no access token")
	}
	return &rr, nil
}

// Logout calls POST /api/logout to revoke the session behind refreshToken.
func (c *Client) Logout(ctx context.Context, refreshToken, accessToken string) error {
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	url := c.BaseURL + "/api/logout"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return parseErrorResponse(resp)
}

// ListTeamModels calls GET /api/teams/{team_id}/llm/models and returns the
// model aliases the team may use through the managed gateway.
//
// The reply names aliases only. Which provider serves one, and with whose
// credential, stays on the server.
func (c *Client) ListTeamModels(ctx context.Context, token, teamID string) ([]llmwire.Model, error) {
	if teamID == "" {
		return nil, errors.New("team is required")
	}
	url := c.BaseURL + fmt.Sprintf(llmwire.ModelsPath, teamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}
	var out llmwire.ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out.Models, nil
}

// parseErrorResponse reads an error JSON body ({"error":"..."} or {"message":"..."})
// and returns it as an error with the HTTP status code.
func parseErrorResponse(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var errBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &errBody) == nil {
		msg := errBody.Error
		if msg == "" {
			msg = errBody.Message
		}
		if msg != "" {
			return fmt.Errorf("server %d: %s", resp.StatusCode, msg)
		}
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}
