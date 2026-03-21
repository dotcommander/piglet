// Package gittool is an example piglet extension that adds a git_status tool.
//
// This file demonstrates how to register tools that the LLM can invoke.
// Tools provide a JSON schema so the LLM knows the parameters, and an
// Execute function that runs when called.
package gittool

import (
	"context"
	"fmt"
	"os/exec"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"strings"
)

// Register is the extension entry point.
func Register(app *ext.App) error {
	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "git_status",
			Description: "Show the current git status of the working directory",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		PromptHint: "Check git status to understand the current state of the repository",
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			cmd := exec.CommandContext(ctx, "git", "status", "--short")
			cmd.Dir = app.CWD()
			out, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("git status: %w", err)
			}

			text := strings.TrimSpace(string(out))
			if text == "" {
				text = "Working tree clean"
			}

			return &core.ToolResult{
				Content: []core.ContentBlock{core.TextContent{Text: text}},
			}, nil
		},
	})

	app.RegisterTool(&ext.ToolDef{
		ToolSchema: core.ToolSchema{
			Name:        "git_diff",
			Description: "Show the git diff of unstaged changes",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"staged": map[string]any{
						"type":        "boolean",
						"description": "If true, show staged changes instead of unstaged",
					},
				},
			},
		},
		PromptHint: "View code changes before committing",
		Execute: func(ctx context.Context, id string, args map[string]any) (*core.ToolResult, error) {
			gitArgs := []string{"diff"}
			if staged, ok := args["staged"].(bool); ok && staged {
				gitArgs = append(gitArgs, "--cached")
			}

			cmd := exec.CommandContext(ctx, "git", gitArgs...)
			cmd.Dir = app.CWD()
			out, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("git diff: %w", err)
			}

			text := strings.TrimSpace(string(out))
			if text == "" {
				text = "No changes"
			}

			// Truncate if too long
			if len(text) > 4000 {
				text = text[:4000] + "\n... (truncated)"
			}

			return &core.ToolResult{
				Content: []core.ContentBlock{core.TextContent{Text: text}},
			}, nil
		},
	})

	return nil
}
