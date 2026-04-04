package mcpservers

import (
	"encoding/json"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func formatCallToolResult(res *mcpsdk.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		parts = append(parts, formatContent(c))
	}
	s := strings.TrimSpace(strings.Join(parts, "\n"))
	if res.StructuredContent != nil {
		b, err := json.MarshalIndent(res.StructuredContent, "", "  ")
		if err == nil && len(b) > 0 {
			if s != "" {
				s += "\n\n"
			}
			s += "structured_content:\n" + string(b)
		}
	}
	if s == "" && !res.IsError {
		s = "(empty tool result)"
	}
	return strings.TrimSpace(s)
}

func formatContent(c mcpsdk.Content) string {
	switch x := c.(type) {
	case *mcpsdk.TextContent:
		return x.Text
	default:
		b, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprintf("%v", c)
		}
		return string(b)
	}
}
