// Package tools provides concrete agent tools (e.g. read_file, write_file, webfetch).
package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"buildmax/internal/llm"

	"github.com/k3a/html2text"
)

const (
	// MaxContentRunes is the maximum runes of fetched content before truncation.
	MaxContentRunes = 200_000
	// fetchTimeout is the HTTP client timeout for fetches.
	fetchTimeout = 30 * time.Second
	// maxCacheEntries is the upper bound on cached URL results.
	maxCacheEntries = 100
)

// errRedirectToOtherHost is returned by CheckRedirect when redirect goes to a different host.
type errRedirectToOtherHost struct{ redirectURL string }

func (e errRedirectToOtherHost) Error() string { return "redirect to different host: " + e.redirectURL }

// cacheEntry holds a cached result and its expiry.
type cacheEntry struct {
	result    string
	expiresAt time.Time
}

// WebFetch is a tool that fetches a URL, converts HTML to markdown, optionally
// processes content with the LLM using a prompt, and returns the result.
// It implements the core.Tool interface.
type WebFetch struct {
	caller   llm.LLMCaller
	cache    map[string]cacheEntry
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
}

// NewWebFetch creates a WebFetch tool with the given LLM caller and cache TTL.
// caller must be non-nil.
func NewWebFetch(caller llm.LLMCaller, cacheTTL time.Duration) (*WebFetch, error) {
	if caller == nil {
		return nil, errors.New("WebFetch: caller is required")
	}
	return &WebFetch{
		caller:   caller,
		cache:    make(map[string]cacheEntry),
		cacheTTL: cacheTTL,
	}, nil
}

// Name returns the tool name for the LLM.
func (w *WebFetch) Name() string { return ToolNameWebFetch }

// Description returns a short description so the LLM knows when to use this tool.
func (w *WebFetch) Description() string {
	return "Fetch content from a URL and optionally process it with a prompt. Converts HTML to markdown. Use when you need to retrieve and analyze web content. Prefer an MCP web fetch tool if available. Read-only; content may be truncated if large. If the URL redirects to a different host, the tool returns the redirect URL so you can call WebFetch again with it. Results are cached for 15 minutes."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (w *WebFetch) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch content from (HTTP is upgraded to HTTPS)",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Optional prompt to run on the fetched content (e.g. what to extract or summarize); omit to return raw content",
			},
		},
		"required": []string{"url"},
	}
}

// Execute fetches the URL, converts HTML to markdown, optionally calls the LLM with content+prompt, and returns the result.
func (w *WebFetch) Execute(ctx context.Context, args map[string]any) (string, error) {
	// 1. Parse args
	vURL, ok := args["url"]
	if !ok {
		return "", errors.New("missing url")
	}
	rawURL, ok := vURL.(string)
	if !ok {
		return "", errors.New("url must be a string")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("url is empty")
	}

	var prompt string
	if v, ok := args["prompt"]; ok {
		if s, ok := v.(string); ok {
			prompt = strings.TrimSpace(s)
		}
	}

	// 2. Normalize URL: http -> https
	normalizedURL, err := normalizeURL(rawURL)
	if err != nil {
		return "", err
	}

	// 3. Cache lookup (by normalized request URL)
	w.cacheMu.RLock()
	ent, ok := w.cache[normalizedURL]
	w.cacheMu.RUnlock()
	if ok && time.Now().Before(ent.expiresAt) {
		return ent.result, nil
	}
	if ok {
		w.cacheMu.Lock()
		delete(w.cache, normalizedURL)
		w.cacheMu.Unlock()
	}

	// 4. HTTP fetch with custom CheckRedirect
	client := w.httpClient(normalizedURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizedURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		var redirectErr errRedirectToOtherHost
		if errors.As(err, &redirectErr) {
			return "Redirected to a different host. Fetch this URL instead: " + redirectErr.redirectURL, nil
		}
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("fetch failed: HTTP " + resp.Status)
	}

	// 5. Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyStr := string(body)

	// 6. Convert to text (HTML -> markdown or plain text)
	content := convertToText(bodyStr, resp.Header.Get("Content-Type"))

	// 7. Truncate
	content = truncateContent(content, MaxContentRunes)

	// 8. Optional LLM step
	finalURL := resp.Request.URL.String()
	if prompt != "" {
		reply, _, _, err := w.caller.ChatWithTools(ctx, []llm.Message{
			{Role: "system", Content: "Answer based only on the following content.\n\n" + content},
			{Role: "user", Content: prompt},
		}, nil)
		if err != nil {
			return "", err
		}
		content = reply
	}

	// 9. Cache store (key = final request URL) and return
	w.cacheMu.Lock()
	w.evictExpired()
	if len(w.cache) >= maxCacheEntries {
		w.evictOldest()
	}
	w.cache[finalURL] = cacheEntry{result: content, expiresAt: time.Now().Add(w.cacheTTL)}
	w.cacheMu.Unlock()
	return content, nil
}

// evictExpired removes all expired cache entries. Must be called with cacheMu held.
func (w *WebFetch) evictExpired() {
	now := time.Now()
	for k, e := range w.cache {
		if now.After(e.expiresAt) {
			delete(w.cache, k)
		}
	}
}

// evictOldest removes the cache entry with the earliest expiresAt. Must be called with cacheMu held.
func (w *WebFetch) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, e := range w.cache {
		if first || e.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.expiresAt
			first = false
		}
	}
	if !first {
		delete(w.cache, oldestKey)
	}
}

// normalizeURL returns the URL with scheme set to https if it was http.
// Localhost (127.0.0.1, localhost) is left as http so tests can use httptest.NewServer().
func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("invalid url: " + err.Error())
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	host := strings.ToLower(u.Hostname())
	isLocalhost := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if u.Scheme == "http" && !isLocalhost {
		u.Scheme = "https"
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("url must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("url has no host")
	}
	return u.String(), nil
}

// httpClient returns an HTTP client that rejects cross-host redirects.
func (w *WebFetch) httpClient(initialURL string) *http.Client {
	initialHost := ""
	if u, err := url.Parse(initialURL); err == nil {
		initialHost = u.Host
	}
	return &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			origHost := via[0].URL.Host
			if origHost == "" {
				origHost = initialHost
			}
			if req.URL.Host != origHost {
				return errRedirectToOtherHost{redirectURL: req.URL.String()}
			}
			return nil
		},
	}
}

// convertToText converts HTML to plain text (markdown-flavored) or returns body as-is for non-HTML.
func convertToText(body, contentType string) string {
	isHTML := strings.Contains(strings.ToLower(contentType), "html") ||
		strings.HasPrefix(strings.TrimSpace(body), "<!")
	if !isHTML {
		return body
	}
	return html2text.HTML2Text(body)
}

// truncateContent truncates content to maxRunes and appends a note if truncated.
func truncateContent(content string, maxRunes int) string {
	if utf8.RuneCountInString(content) <= maxRunes {
		return content
	}
	runes := []rune(content)
	n := len(runes)
	return string(runes[:maxRunes]) + "\n(content truncated; total " + strconv.Itoa(n) + " characters)"
}
