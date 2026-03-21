package main

import (
	"context"
	"fmt"
	"os"
	"maps"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/memory"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/tool"
	"github.com/dotcommander/piglet/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Determine prompt: args after flags or stdin
	userPrompt := strings.Join(os.Args[1:], " ")
	interactive := userPrompt == ""

	// Load config
	settings, _ := config.Load() // best effort

	// Auth
	auth, err := config.NewAuthDefault()
	if err != nil {
		return fmt.Errorf("init auth: %w", err)
	}

	// Registry and model resolution
	registry := provider.NewRegistry()
	modelQuery := config.Resolve("DEFAULT_MODEL", settings.DefaultModel, "gpt-4o")

	model, ok := registry.Resolve(modelQuery)
	if !ok {
		return fmt.Errorf("unknown model: %s", modelQuery)
	}

	// Create provider
	apiKeyFn := func() string {
		return auth.GetAPIKey(model.Provider)
	}
	prov, err := registry.Create(model, apiKeyFn)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}

	// Working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}

	// Session directory
	sessDir, _ := config.SessionsDir()

	// Extension app + built-in tools + commands
	app := ext.NewApp(cwd)
	tool.RegisterBuiltins(app, tool.BashConfig{
		DefaultTimeout: settings.Bash.DefaultTimeout,
		MaxTimeout:     settings.Bash.MaxTimeout,
		MaxStdout:      settings.Bash.MaxStdout,
		MaxStderr:      settings.Bash.MaxStderr,
	})
	command.RegisterBuiltins(app, settings.Shortcuts)
	app.RegisterExtInfo(ext.ExtInfo{
		Name:     "builtin",
		Kind:     "builtin",
		Runtime:  "go",
		Tools:    app.Tools(),
		Commands: slices.Sorted(maps.Keys(app.Commands())),
	})

	// Project memory (tools, /memory command, prompt section)
	memory.Register(app)

	// Git context (diff + recent commits in system prompt)
	prompt.RegisterGitContext(app, prompt.GitContextConfig{
		MaxDiffStatFiles: settings.Git.MaxDiffStatFiles,
		MaxLogLines:      settings.Git.MaxLogLines,
		MaxDiffHunkLines: settings.Git.MaxDiffHunkLines,
		CommandTimeout:   time.Duration(settings.Git.CommandTimeout) * time.Second,
	})

	// External extensions (TypeScript, Python, etc.)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	extCleanup, _ := external.LoadAll(ctx, app)
	defer extCleanup()

	// System prompt: config.yaml systemPrompt → fallback default
	basePrompt := settings.SystemPrompt
	if basePrompt == "" {
		basePrompt = "You are piglet, a helpful coding assistant."
	}
	system := prompt.Build(app, basePrompt, prompt.BuildOptions{
		OrderOverrides: settings.PromptOrder,
	})

	// Session
	sess, err := session.New(sessDir, cwd)
	if err != nil {
		// Non-fatal: continue without persistence
		sess = nil
	}
	if sess != nil {
		defer sess.Close()
		sess.SetModel(model.ID)
	}

	// Agent
	coreTools := app.CoreTools()
	compactAt := config.IntOr(settings.Agent.CompactAt, 0)
	ag := core.NewAgent(core.AgentConfig{
		Provider:    prov,
		System:      system,
		Tools:       coreTools,
		MaxTurns:    config.IntOr(settings.Agent.MaxTurns, 10),
		MaxMessages: config.IntOr(settings.Agent.MaxMessages, 200),
		CompactAt:   compactAt,
		OnCompact:   makeCompactor(prov),
	})

	// Bind domain managers so commands work through ext.App
	sessPtr := &sess
	app.Bind(ag,
		ext.WithSessionManager(&sessionMgr{dir: sessDir, current: sessPtr}),
		ext.WithModelManager(&modelMgr{registry: registry, auth: auth}),
	)

	if interactive {
		return tui.Run(tui.Config{
			Agent:    ag,
			Session:  sess,
			Model:    model,
			Models:   registry.Models(),
			SessDir:  sessDir,
			Theme:    tui.DefaultTheme(),
			App:      app,
			Settings: &settings,
		})
	}

	return runPrint(ag, sess, userPrompt)
}

func runPrint(ag *core.Agent, sess *session.Session, userPrompt string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Persist user message first
	if sess != nil {
		_ = sess.Append(&core.UserMessage{Content: userPrompt})
	}

	ch := ag.Start(ctx, userPrompt)

	var agentErr error

	for evt := range ch {
		switch e := evt.(type) {
		case core.EventStreamDelta:
			fmt.Print(e.Delta)
		case core.EventToolStart:
			fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", e.ToolName)
		case core.EventToolEnd:
			if e.IsError {
				fmt.Fprintf(os.Stderr, "[tool error: %s]\n", e.ToolName)
			}
		case core.EventRetry:
			fmt.Fprintf(os.Stderr, "[retry %d/%d: %s]\n", e.Attempt, e.Max, e.Error)
		case core.EventCompact:
			fmt.Fprintf(os.Stderr, "[compacted: %d → %d messages at %d tokens]\n", e.Before, e.After, e.TokensAtCompact)
		case core.EventAgentEnd:
			fmt.Println()
		}

		if e, ok := evt.(core.EventTurnEnd); ok {
			if e.Assistant != nil && (e.Assistant.StopReason == core.StopReasonError || e.Assistant.StopReason == core.StopReasonAborted) {
				msg := e.Assistant.Error
				if msg == "" {
					msg = string(e.Assistant.StopReason)
				}
				agentErr = fmt.Errorf("agent error: %s", msg)
			}
			if sess != nil {
				if e.Assistant != nil {
					_ = sess.Append(e.Assistant)
				}
				for _, tr := range e.ToolResults {
					_ = sess.Append(tr)
				}
			}
		}
	}

	return agentErr
}

// makeCompactor returns an OnCompact callback that summarizes messages via the LLM.
func makeCompactor(prov core.StreamProvider) func([]core.Message) (string, error) {
	return func(msgs []core.Message) (string, error) {
		// Build a text representation of the middle messages
		var b strings.Builder
		for _, m := range msgs {
			switch msg := m.(type) {
			case *core.UserMessage:
				fmt.Fprintf(&b, "User: %s\n", msg.Content)
			case *core.AssistantMessage:
				for _, c := range msg.Content {
					if tc, ok := c.(core.TextContent); ok {
						fmt.Fprintf(&b, "Assistant: %s\n", tc.Text)
					}
				}
			case *core.ToolResultMessage:
				for _, c := range msg.Content {
					if tc, ok := c.(core.TextContent); ok {
						text := tc.Text
						if len(text) > 200 {
							r := []rune(text)
							if len(r) > 200 {
								text = string(r[:200]) + "..."
							}
						}
						fmt.Fprintf(&b, "Tool(%s): %s\n", msg.ToolCallID, text)
					}
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ch := prov.Stream(ctx, core.StreamRequest{
			System: "Summarize this conversation excerpt concisely. Preserve key decisions, file paths, errors, and outcomes. Output only the summary, no preamble.",
			Messages: []core.Message{
				&core.UserMessage{Content: b.String(), Timestamp: time.Now()},
			},
		})

		var summary strings.Builder
		for evt := range ch {
			if evt.Type == core.StreamTextDelta {
				summary.WriteString(evt.Delta)
			}
			if evt.Type == core.StreamError {
				return "", evt.Error
			}
		}

		result := summary.String()
		if result == "" {
			return "", fmt.Errorf("empty summary")
		}
		return "[Conversation compacted]\n\n" + result, nil
	}
}
