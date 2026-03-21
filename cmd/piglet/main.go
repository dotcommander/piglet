package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"maps"
	"os/signal"
	"path/filepath"
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
	"github.com/dotcommander/piglet/rtk"
	"github.com/dotcommander/piglet/safeguard"
	"github.com/dotcommander/piglet/skill"
	"github.com/dotcommander/piglet/subagent"
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
	// Handle flags before anything else
	var debug bool
	var promptArgs []string
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--help", "-h":
			printHelp()
			return nil
		case "--version", "-v":
			fmt.Println("piglet dev")
			return nil
		case "--debug":
			debug = true
		default:
			promptArgs = append(promptArgs, arg)
		}
	}

	// Determine prompt: args after flags or stdin
	userPrompt := strings.Join(promptArgs, " ")
	interactive := userPrompt == ""

	// Load config
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Auth
	auth, err := config.NewAuthDefault()
	if err != nil {
		return fmt.Errorf("init auth: %w", err)
	}

	// Registry and model resolution
	registry := provider.NewRegistry()
	modelQuery := os.Getenv("PIGLET_DEFAULT_MODEL")
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return fmt.Errorf("no default model configured\nSet defaultModel in ~/.config/piglet/config.yaml or PIGLET_DEFAULT_MODEL env var\nRun: piglet /config --setup")
	}

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

	// Debug logging (--debug flag or debug: true in config)
	if debug || settings.Debug {
		logger, cleanup, err := openDebugLog()
		if err != nil {
			return fmt.Errorf("debug log: %w", err)
		}
		defer cleanup()
		if d, ok := prov.(provider.Debuggable); ok {
			d.SetLogger(logger)
		}
		logger.Info("session start", "model", model.ID, "provider", model.Provider)
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
	}, tool.ToolConfig{
		ReadLimit: settings.Tools.ReadLimit,
		GrepLimit: settings.Tools.GrepLimit,
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

	// Core behavioral guidelines (tool usage, output style, safety)
	prompt.RegisterBehavior(app)

	// Project docs (configurable context files → system prompt)
	prompt.RegisterProjectDocs(app, settings.ProjectDocs)

	// Safeguard: block dangerous bash commands (enabled by default)
	safeguard.Register(app, settings.Safeguard)

	// RTK token optimization (auto-detects rtk in PATH)
	rtk.Register(app, settings.RTK)

	// Skills: on-demand methodology loading from ~/.config/piglet/skills/
	skill.Register(app)

	// Sub-agent delegation (dispatch tool)
	subagent.Register(app, subagent.Config{
		MaxTurns: config.IntOr(settings.SubAgent.MaxTurns, 10),
	})

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

	// Self-knowledge (dynamic tools/commands/shortcuts listing)
	prompt.RegisterSelfKnowledge(app)

	// System prompt: config.yaml systemPrompt or prompt.md (no hardcoded fallback)
	basePrompt := settings.SystemPrompt
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

	// Register LLM-powered compactor extension
	command.RegisterCompactor(app, prov, compactAt)

	// Build agent config, pulling compactor from ext if registered
	var opts core.StreamOptions
	if settings.Agent.MaxTokens > 0 {
		mt := settings.Agent.MaxTokens
		opts.MaxTokens = &mt
	}
	agCfg := core.AgentConfig{
		Provider:    prov,
		System:      system,
		Tools:       coreTools,
		Options:     opts,
		MaxTurns:        config.IntOr(settings.Agent.MaxTurns, 10),
		MaxMessages:     config.IntOr(settings.Agent.MaxMessages, 200),
		MaxRetries:      settings.Agent.MaxRetries,
		ToolConcurrency: settings.Agent.ToolConcurrency,
	}
	if c := app.Compactor(); c != nil {
		agCfg.CompactAt = c.Threshold
		agCfg.OnCompact = c.Compact
	}
	ag := core.NewAgent(agCfg)

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

	return runPrint(ag, app, sess, userPrompt)
}

func runPrint(ag *core.Agent, app *ext.App, sess *session.Session, userPrompt string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Run message hooks for ephemeral turn context
	if injections, err := app.RunMessageHooks(ctx, userPrompt); err == nil && len(injections) > 0 {
		ag.SetTurnContext(injections)
	}

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
			if e.Assistant != nil {
				u := e.Assistant.Usage
				fmt.Fprintf(os.Stderr, "[tokens: in=%d out=%d", u.InputTokens, u.OutputTokens)
				if u.CacheReadTokens > 0 || u.CacheWriteTokens > 0 {
					fmt.Fprintf(os.Stderr, " cache_read=%d cache_write=%d", u.CacheReadTokens, u.CacheWriteTokens)
				}
				fmt.Fprintln(os.Stderr, "]")

				if e.Assistant.StopReason == core.StopReasonError || e.Assistant.StopReason == core.StopReasonAborted {
					msg := e.Assistant.Error
					if msg == "" {
						msg = string(e.Assistant.StopReason)
					}
					agentErr = fmt.Errorf("agent error: %s", msg)
				}
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

func openDebugLog() (*slog.Logger, func(), error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fmt.Fprintf(os.Stderr, "debug: logging to %s\n", path)
	return logger, func() { f.Close() }, nil
}

func printHelp() {
	fmt.Print(`piglet — minimalist coding assistant

Usage:
  piglet                  Interactive TUI mode
  piglet <prompt>         Single-shot mode (prints response and exits)
  piglet --help           Show this help
  piglet --version        Show version
  piglet --debug          Log all request/response payloads

Interactive commands:
  /help                   List all commands
  /model                  Switch LLM model          (ctrl+p)
  /session                Switch sessions            (ctrl+s)
  /clear                  Clear conversation
  /compact                Compact conversation history
  /config                 Show config paths
  /config --setup         Create default config
  /undo                   Restore files to pre-edit state

Config:
  ~/.config/piglet/config.yaml    Settings
  ~/.config/piglet/auth.json      API keys
  ~/.config/piglet/prompt.md      System prompt
  ~/.config/piglet/models.yaml    Model catalog

Debug:
  ~/.config/piglet/debug.log      Payload log (--debug or debug: true)

Environment:
  PIGLET_DEFAULT_MODEL    Override default model
`)
}

