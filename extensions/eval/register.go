package eval

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/sdk"
)

//go:embed defaults/judge-prompt.md
var defaultJudgePrompt string

//go:embed defaults/example-suite.yaml
var defaultExampleSuite string

// Register wires all eval capabilities into the extension.
func Register(e *sdk.Extension) {
	e.OnInitAppend(func(x *sdk.Extension) {
		_ = SeedDefaults()
	})

	registerCommands(e)
	registerTools(e)
}

func registerCommands(e *sdk.Extension) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "eval",
		Description: "Run evaluation suites: /eval run <suite>, /eval list, /eval results [suite], /eval compare <path1> <path2>",
		Handler: func(ctx context.Context, args string) error {
			sub := strings.TrimSpace(args)
			switch {
			case sub == "" || sub == "list":
				return handleEvalList(e)
			case strings.HasPrefix(sub, "run "):
				return handleEvalRun(ctx, e, strings.TrimPrefix(sub, "run "))
			case strings.HasPrefix(sub, "results"):
				filter := strings.TrimSpace(strings.TrimPrefix(sub, "results"))
				return handleEvalResults(e, filter)
			case strings.HasPrefix(sub, "compare "):
				return handleEvalCompare(e, strings.TrimPrefix(sub, "compare "))
			default:
				e.ShowMessage("Usage: /eval [run <suite>|list|results [suite]|compare <path1> <path2>]")
			}
			return nil
		},
	})
}

func handleEvalList(e *sdk.Extension) error {
	dir, err := suitesDir()
	if err != nil {
		e.ShowMessage("Error: " + err.Error())
		return nil
	}
	summaries, err := ListSuites(dir)
	if err != nil {
		e.ShowMessage("Error listing suites: " + err.Error())
		return nil
	}
	if len(summaries) == 0 {
		e.ShowMessage("No evaluation suites found in " + dir)
		return nil
	}
	var b strings.Builder
	b.WriteString("**Evaluation Suites**\n\n")
	for _, s := range summaries {
		fmt.Fprintf(&b, "- **%s** — %s (%d cases)\n  `%s`\n", s.Name, s.Description, s.CaseCount, s.Path)
	}
	e.ShowMessage(b.String())
	return nil
}

func handleEvalRun(ctx context.Context, e *sdk.Extension, args string) error {
	suiteName := strings.TrimSpace(args)
	if suiteName == "" {
		e.ShowMessage("Usage: /eval run <suite-name>")
		return nil
	}
	dir, err := suitesDir()
	if err != nil {
		e.ShowMessage("Error: " + err.Error())
		return nil
	}
	path := filepath.Join(dir, suiteName+".yaml")
	suite, err := LoadSuite(path)
	if err != nil {
		e.ShowMessage("Error loading suite: " + err.Error())
		return nil
	}
	e.ShowMessage(fmt.Sprintf("Running suite **%s** (%d cases)...", suite.Name, len(suite.Cases)))
	runner := NewRunner(e)
	result, err := runner.Run(ctx, suite, nil)
	if err != nil {
		e.ShowMessage("Error running suite: " + err.Error())
		return nil
	}
	saved, err := SaveResult(result)
	if err != nil {
		e.ShowMessage("Warning: could not save results: " + err.Error())
	}
	e.ShowMessage(formatRunSummary(result, saved))
	return nil
}

func handleEvalResults(e *sdk.Extension, suiteFilter string) error {
	summaries, err := ListResults(suiteFilter)
	if err != nil {
		e.ShowMessage("Error: " + err.Error())
		return nil
	}
	if len(summaries) == 0 {
		msg := "No results found."
		if suiteFilter != "" {
			msg = fmt.Sprintf("No results found for suite %q.", suiteFilter)
		}
		e.ShowMessage(msg)
		return nil
	}
	var b strings.Builder
	b.WriteString("**Evaluation Results**\n\n")
	for _, s := range summaries {
		fmt.Fprintf(&b, "- **%s** — %s — %d/%d passed\n  `%s`\n",
			s.Suite, s.RanAt.Format("2006-01-02 15:04"), s.Passed, s.Total, s.Path)
	}
	e.ShowMessage(b.String())
	return nil
}

func handleEvalCompare(e *sdk.Extension, args string) error {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		e.ShowMessage("Usage: /eval compare <path1> <path2>")
		return nil
	}
	a, err := LoadResult(parts[0])
	if err != nil {
		e.ShowMessage("Error loading first result: " + err.Error())
		return nil
	}
	b, err := LoadResult(parts[1])
	if err != nil {
		e.ShowMessage("Error loading second result: " + err.Error())
		return nil
	}
	comp := Compare(a, b)
	e.ShowMessage("```\n" + comp.Format() + "```")
	return nil
}

func registerTools(e *sdk.Extension) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "eval_run",
		Description: "Run an evaluation suite by name and return the results summary",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"suite": map[string]any{
					"type":        "string",
					"description": "Name of the suite to run (filename without .yaml)",
				},
				"cases": map[string]any{
					"type":        "array",
					"description": "Optional list of case names to run (empty = run all)",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"suite"},
		},
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return toolEvalRun(ctx, e, args)
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "eval_list",
		Description: "List all available evaluation suites",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ map[string]any) (*sdk.ToolResult, error) {
			return toolEvalList()
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "eval_compare",
		Description: "Compare two evaluation run results and show score deltas",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_a": map[string]any{
					"type":        "string",
					"description": "Path to first result JSON file",
				},
				"run_b": map[string]any{
					"type":        "string",
					"description": "Path to second result JSON file",
				},
			},
			"required": []string{"run_a", "run_b"},
		},
		Execute: func(_ context.Context, args map[string]any) (*sdk.ToolResult, error) {
			return toolEvalCompare(args)
		},
	})
}

func toolEvalRun(ctx context.Context, e *sdk.Extension, args map[string]any) (*sdk.ToolResult, error) {
	suiteName, _ := args["suite"].(string)
	if suiteName == "" {
		return sdk.ErrorResult("suite name required"), nil
	}

	var caseFilter []string
	if raw, ok := args["cases"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				caseFilter = append(caseFilter, s)
			}
		}
	}

	dir, err := suitesDir()
	if err != nil {
		return sdk.ErrorResult("resolve suites dir: " + err.Error()), nil
	}

	suite, err := LoadSuite(filepath.Join(dir, suiteName+".yaml"))
	if err != nil {
		return sdk.ErrorResult("load suite: " + err.Error()), nil
	}

	runner := NewRunner(e)
	result, err := runner.Run(ctx, suite, caseFilter)
	if err != nil {
		return sdk.ErrorResult("run suite: " + err.Error()), nil
	}

	saved, _ := SaveResult(result)
	return sdk.TextResult(formatRunSummary(result, saved)), nil
}

func toolEvalList() (*sdk.ToolResult, error) {
	dir, err := suitesDir()
	if err != nil {
		return sdk.ErrorResult("resolve suites dir: " + err.Error()), nil
	}
	summaries, err := ListSuites(dir)
	if err != nil {
		return sdk.ErrorResult("list suites: " + err.Error()), nil
	}
	if len(summaries) == 0 {
		return sdk.TextResult("No evaluation suites found."), nil
	}
	var b strings.Builder
	for _, s := range summaries {
		fmt.Fprintf(&b, "%s: %s (%d cases) — %s\n", s.Name, s.Description, s.CaseCount, s.Path)
	}
	return sdk.TextResult(b.String()), nil
}

func toolEvalCompare(args map[string]any) (*sdk.ToolResult, error) {
	pathA, _ := args["run_a"].(string)
	pathB, _ := args["run_b"].(string)
	if pathA == "" || pathB == "" {
		return sdk.ErrorResult("run_a and run_b are required"), nil
	}
	a, err := LoadResult(pathA)
	if err != nil {
		return sdk.ErrorResult("load run_a: " + err.Error()), nil
	}
	b, err := LoadResult(pathB)
	if err != nil {
		return sdk.ErrorResult("load run_b: " + err.Error()), nil
	}
	comp := Compare(a, b)
	return sdk.TextResult(comp.Format()), nil
}

func formatRunSummary(result *RunResult, savedPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** — %d/%d passed (avg score: %.2f)\n\n",
		result.Suite, result.Summary.Passed, result.Summary.Total, result.Summary.AvgScore)
	for _, c := range result.Cases {
		status := "PASS"
		if !c.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "[%s] %s (%.2f) — %s\n", status, c.Name, c.Score, c.Reason)
	}
	if savedPath != "" {
		fmt.Fprintf(&b, "\nSaved: `%s`", savedPath)
	}
	return b.String()
}
