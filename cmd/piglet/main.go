package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/tool"
	"github.com/dotcommander/piglet/tui"
	"strings"
	"syscall"
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
	tool.RegisterBuiltins(app)
	command.RegisterBuiltins(app, registry.Models(), sessDir)

	// System prompt: config.yaml systemPrompt → fallback default
	basePrompt := settings.SystemPrompt
	if basePrompt == "" {
		basePrompt = "You are piglet, a helpful coding assistant."
	}
	system := prompt.Build(app, basePrompt)

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
	ag := core.NewAgent(core.AgentConfig{
		Provider: prov,
		System:   system,
		Tools:    coreTools,
		MaxTurns: 10,
	})

	if interactive {
		return tui.Run(tui.Config{
			Agent:   ag,
			Session: sess,
			Model:   model,
			Models:  registry.Models(),
			SessDir: sessDir,
			Theme:   tui.DefaultTheme(),
			App:     app,
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
