package mcp

import (
	"context"
	"strings"

	gomcp "github.com/mark3labs/mcp-go/mcp"
)

// ToolCallResult holds the extracted text and error status from an MCP tool call.
type ToolCallResult struct {
	Text    string
	IsError bool
}

// CallAndExtract invokes an MCP tool and extracts the text result.
func CallAndExtract(ctx context.Context, client *Client, toolName string, args map[string]any) ToolCallResult {
	result, err := client.CallTool(ctx, toolName, args)
	if err != nil {
		return ToolCallResult{Text: "mcp error: " + err.Error(), IsError: true}
	}

	text := ExtractText(result)
	return ToolCallResult{Text: text, IsError: result.IsError}
}

// ExtractText concatenates all TextContent from a CallToolResult.
func ExtractText(result *gomcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if tc, ok := gomcp.AsTextContent(c); ok {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(tc.Text)
		}
	}
	if sb.Len() == 0 {
		return "(no text content)"
	}
	return sb.String()
}
