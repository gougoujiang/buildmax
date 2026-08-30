// Command mcp is a small MCP server for local testing of BuildMax MCP integration.
// Supports stdio (default), SSE, and streamable HTTP transports.
//
// Usage:
//
//	go run ./tools/mcp                  # stdio
//	go run ./tools/mcp --mode sse --addr :8080
//	go run ./tools/mcp --mode streamable-http --addr :8081
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	mode := flag.String("mode", "stdio", "transport mode: stdio | sse | streamable-http")
	addr := flag.String("addr", ":8080", "listen address for sse or streamable-http modes")
	flag.Parse()

	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mcp", Version: "0.0.1"}, nil)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "echo",
		Description: "Echo back a message with optional uppercase/repeat controls for quick MCP call testing.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Text to echo back",
				},
				"uppercase": map[string]any{
					"type":        "boolean",
					"description": "When true, return the echoed text in uppercase.",
				},
				"repeat": map[string]any{
					"type":        "integer",
					"description": "Optional repeat count from 1 to 5.",
					"minimum":     1,
					"maximum":     5,
				},
			},
			"required": []string{"message"},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		msg, _ := in["message"].(string)
		uppercase, _ := in["uppercase"].(bool)
		repeat, err := intFromInput(in["repeat"], 1)
		if err != nil {
			return nil, nil, err
		}
		if repeat < 1 || repeat > 5 {
			return nil, nil, fmt.Errorf("repeat must be between 1 and 5")
		}
		if uppercase {
			msg = strings.ToUpper(msg)
		}
		parts := make([]string, repeat)
		for i := range parts {
			parts[i] = msg
		}
		return nil, map[string]any{
			"echo":            strings.Join(parts, " "),
			"repeat":          repeat,
			"uppercase":       uppercase,
			"mcp_demo_custom": os.Getenv("MCP_DEMO_CUSTOM"),
		}, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "text_stats",
		Description: "Analyze a text block and return counts useful for structured MCP testing.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Text to analyze.",
				},
			},
			"required": []string{"text"},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		text, _ := in["text"].(string)
		trimmed := strings.TrimSpace(text)
		lines := 0
		if trimmed != "" {
			lines = len(strings.Split(trimmed, "\n"))
		}
		words := len(strings.Fields(text))
		runes := []rune(text)
		return nil, map[string]any{
			"characters":        len(runes),
			"characters_no_ws":  countNonWhitespace(runes),
			"words":             words,
			"lines":             lines,
			"preview":           truncateText(trimmed, 80),
			"contains_todo":     strings.Contains(strings.ToLower(text), "todo"),
			"contains_markdown": strings.Contains(text, "#") || strings.Contains(text, "- ") || strings.Contains(text, "```"),
		}, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "sum_numbers",
		Description: "Calculate summary statistics for a list of numbers.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"numbers": map[string]any{
					"type":        "array",
					"description": "List of numbers to summarize.",
					"items": map[string]any{
						"type": "number",
					},
					"minItems": 1,
				},
			},
			"required": []string{"numbers"},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in map[string]any) (*mcpsdk.CallToolResult, map[string]any, error) {
		nums, err := floatSliceFromInput(in["numbers"])
		if err != nil {
			return nil, nil, err
		}
		if len(nums) == 0 {
			return nil, nil, fmt.Errorf("numbers must not be empty")
		}
		sum := 0.0
		minVal := nums[0]
		maxVal := nums[0]
		for _, n := range nums {
			sum += n
			minVal = math.Min(minVal, n)
			maxVal = math.Max(maxVal, n)
		}
		return nil, map[string]any{
			"count":   len(nums),
			"sum":     sum,
			"average": sum / float64(len(nums)),
			"min":     minVal,
			"max":     maxVal,
		}, nil
	})

	switch *mode {
	case "stdio":
		if err := srv.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
			log.Fatal(err)
		}
	case "sse":
		handler := mcpsdk.NewSSEHandler(func(_ *http.Request) *mcpsdk.Server { return srv }, nil)
		log.Printf("mcp: SSE mode listening on %s", *addr)
		if err := http.ListenAndServe(*addr, handler); err != nil {
			log.Fatal(err)
		}
	case "streamable-http":
		handler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server { return srv }, nil)
		log.Printf("mcp: streamable HTTP mode listening on %s", *addr)
		if err := http.ListenAndServe(*addr, handler); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown mode %q: must be stdio, sse, or streamable-http", *mode)
	}
}

func intFromInput(v any, fallback int) (int, error) {
	if v == nil {
		return fallback, nil
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("expected integer value")
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("expected integer value")
	}
}

func floatSliceFromInput(v any) ([]float64, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("numbers must be an array")
	}
	out := make([]float64, 0, len(raw))
	for _, item := range raw {
		switch n := item.(type) {
		case float64:
			out = append(out, n)
		case int:
			out = append(out, float64(n))
		case int32:
			out = append(out, float64(n))
		case int64:
			out = append(out, float64(n))
		default:
			return nil, fmt.Errorf("numbers must contain only numeric values")
		}
	}
	return out, nil
}

func countNonWhitespace(runes []rune) int {
	count := 0
	for _, r := range runes {
		if !strings.ContainsRune(" \t\r\n", r) {
			count++
		}
	}
	return count
}

func truncateText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max-3]) + "..."
}
