package webfetch

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	"github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/prompt.md
var defaultPrompt string

// Register registers the webfetch extension's tools and prompt section.
func Register(e *sdk.Extension) {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("webfetch: failed to load config", "error", err)
	}

	client := NewWithConfig(cfg)

	e.RegisterPromptSection(sdk.PromptSectionDef{
		Title:   "Web Access",
		Content: xdg.LoadOrCreateExt("webfetch", "prompt.md", strings.TrimSpace(defaultPrompt)),
		Order:   85,
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "web_fetch",
		Description: "Fetch a URL and return its text content. By default uses a reader that converts HTML to clean markdown. Set raw=true to return the raw HTML.",
Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to fetch.",
				},
				"raw": map[string]any{
					"type":        "boolean",
					"description": "If true, return raw HTML instead of extracted text. Default: false.",
				},
			},
			"required": []string{"url"},
		},
		PromptHint: "Fetch and read web pages",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			rawURL, _ := args["url"].(string)
			if rawURL == "" {
				return sdk.ErrorResult("url is required"), nil
			}
			raw, _ := args["raw"].(bool)

			content, err := client.Fetch(ctx, rawURL, raw)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("fetch failed: %s", err)), nil
			}
			return sdk.TextResult(content), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "web_search",
		Description: "Search the web and return results as a markdown list with titles, URLs, and snippets.",
Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return. Default: 5.",
				},
			},
			"required": []string{"query"},
		},
		PromptHint: "Search the web for information",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return sdk.ErrorResult("query is required"), nil
			}
			limit := 5
			if v, ok := args["limit"].(float64); ok && int(v) > 0 {
				limit = int(v)
			}

			results, err := client.Search(ctx, query, limit)
			if err != nil {
				return sdk.ErrorResult(fmt.Sprintf("search failed: %s", err)), nil
			}
			return sdk.TextResult(FormatResults(results)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "webfetch_get_stored",
		Description: "Retrieve cached content from a previous web_fetch or web_search call. Useful when content was truncated in the original response.",
Deferred:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to retrieve (for fetch results).",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Query to retrieve (for search results).",
				},
			},
		},
		PromptHint: "Retrieve cached web content",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			url, _ := args["url"].(string)
			query, _ := args["query"].(string)

			if url != "" {
				content := client.GetStorage().GetFetch(url)
				if content == "" {
					return sdk.ErrorResult("URL not found in cache"), nil
				}
				return sdk.TextResult(content), nil
			}
			if query != "" {
				results := client.GetStorage().GetSearch(query)
				if results == nil {
					return sdk.ErrorResult("Query not found in cache"), nil
				}
				return sdk.TextResult(FormatResults(results)), nil
			}
			return sdk.ErrorResult("url or query is required"), nil
		},
	})
}
