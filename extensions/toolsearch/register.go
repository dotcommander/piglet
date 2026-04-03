// Package toolsearch provides a tool_search tool for finding extensions by name.
// It is registered via packs/code and has no standalone binary.
package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register wires the tool_search tool into a shared SDK extension.
func Register(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "tool_search",
		Description: "Look up the full schema and description for a deferred tool by name. Returns complete parameter definitions so you can call the tool correctly.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Tool name or comma-separated list of tool names to look up.",
				},
			},
			"required": []string{"query"},
		},
		PromptHint: "Look up deferred tool schemas",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return sdk.ErrorResult("query is required"), nil
			}

			tools, err := e.ListHostTools(ctx, "all")
			if err != nil {
				return nil, fmt.Errorf("listing tools: %w", err)
			}

			// Parse comma-separated names
			want := splitNames(query)

			// Build index by name
			byName := make(map[string]*sdk.HostToolInfo, len(tools))
			for i := range tools {
				byName[tools[i].Name] = &tools[i]
			}

			var b strings.Builder
			for _, name := range want {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				info, ok := byName[name]
				if !ok {
					b.WriteString(fmt.Sprintf("## %s\n\nNot found.\n\n", name))
					continue
				}
				b.WriteString(fmt.Sprintf("## %s\n\n", info.Name))
				if info.Description != "" {
					b.WriteString(info.Description)
					b.WriteString("\n\n")
				}
				if info.Parameters != nil {
					schema, err := json.MarshalIndent(info.Parameters, "", "  ")
					if err != nil {
						schema = []byte(fmt.Sprintf("%v", info.Parameters))
					}
					b.WriteString("Parameters:\n```json\n")
					b.Write(schema)
					b.WriteString("\n```\n\n")
				}
			}

			if b.Len() == 0 {
				return sdk.ErrorResult(fmt.Sprintf("no tools found matching: %s", query)), nil
			}

			return sdk.TextResult(b.String()), nil
		},
	})
}

// splitNames splits a comma-separated query into individual names.
func splitNames(query string) []string {
	parts := strings.Split(query, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	sort.Strings(parts)
	return parts
}
