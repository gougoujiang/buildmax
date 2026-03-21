package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// LoginUser is the user subset returned in a login response.
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// LoginResponse is the successful result of POST /api/login.
type LoginResponse struct {
	Token string    `json:"token"`
	User  LoginUser `json:"user"`
}

// Client is a stateless HTTP client for the BuildMax server auth endpoints.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates an auth Client for the given server base URL.
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

// Login calls POST /api/login and returns the token and user on success.
func (c *Client) Login(ctx context.Context, email, otp string) (*LoginResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"email": email,
		"otp":   otp,
	})
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
