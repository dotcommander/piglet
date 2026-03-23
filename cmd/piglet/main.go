package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"runtime/debug"
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
	"github.com/dotcommander/piglet/modelsdev"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/tool"
	"github.com/dotcommander/piglet/tui"
)

// version is set at build time via -ldflags. Falls back to VCS info from
// debug.ReadBuildInfo (works with go install).
var version = "dev"

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value[:min(8, len(s.Value))]
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return "dev-" + rev + dirty
	}
	return "dev"
}

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
			fmt.Println("piglet " + resolveVersion())
			return nil
		case "--debug":
			debug = true
		default:
			promptArgs = append(promptArgs, arg)
		}
	}

	// writeModels tries models.dev API first, falls back to hardcoded default.
	writeModels := func(path string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		yaml, err := modelsdev.GenerateModelsYAML(ctx)
		if err == nil {
			return provider.WriteModelsData(path, yaml)
		}
		fmt.Fprintf(os.Stderr, "  models.dev unavailable, using defaults: %v\n", err)
		return provider.WriteDefaultModels(path)
	}

	// Subcommands (after flag parsing so --debug init works)
	if len(promptArgs) == 1 && promptArgs[0] == "init" {
		return config.RunSetup(writeModels)
	}

	// Determine prompt: args after flags or stdin
	userPrompt := strings.Join(promptArgs, " ")
	interactive := userPrompt == ""

	// First-run setup: auto-detect missing config and run interactive setup
	if config.NeedsSetup() {
		if err := config.RunSetup(writeModels); err != nil {
			return fmt.Errorf("setup: %w", err)
		}
	}

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

	// Background refresh of model catalog from models.dev (every 24h)
	if modelsdev.CacheStale() {
		go func() {
			rCtx, rCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer rCancel()
			if err := modelsdev.RefreshCache(rCtx, registry); err != nil {
				fmt.Fprintf(os.Stderr, "models.dev refresh: %v\n", err)
			}
		}()
	}

	modelQuery := os.Getenv("PIGLET_DEFAULT_MODEL")
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return fmt.Errorf("no default model configured\nSet defaultModel in ~/.config/piglet/config.yaml or PIGLET_DEFAULT_MODEL env var\nRun: piglet init")
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
	command.RegisterPrompts(app)
	app.RegisterExtInfo(ext.ExtInfo{
		Name:     "builtin",
		Kind:     "builtin",
		Runtime:  "go",
		Tools:    app.Tools(),
		Commands: slices.Sorted(maps.Keys(app.Commands())),
	})

	// Core behavioral guidelines (tool usage, output style, safety)
	prompt.RegisterBehavior(app)

	// Project docs (configurable context files → system prompt)
	prompt.RegisterProjectDocs(app, settings.ProjectDocs)

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
	loaded, extCleanup, _ := external.LoadAll(ctx, app)
	defer extCleanup()

	if interactive && loaded == 0 {
		if err := command.InstallOfficialExtensions(os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "auto-install failed: %v\n", err)
		}
		// Reload after install attempt (picks up freshly built extensions)
		loaded, cleanup, _ := external.LoadAll(ctx, app)
		defer cleanup()
		if loaded > 0 {
			fmt.Fprintf(os.Stderr, "Loaded %d extensions.\n\n", loaded)
		}
	}

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
	// Build agent config, pulling compactor from ext if registered
	var opts core.StreamOptions
	if settings.Agent.MaxTokens > 0 {
		mt := settings.Agent.MaxTokens
		opts.MaxTokens = &mt
	}
	agCfg := core.AgentConfig{
		Provider:        prov,
		System:          system,
		Tools:           coreTools,
		Options:         opts,
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
		// Dispatch event to registered handlers (event bus)
		app.DispatchEvent(ctx, evt)

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

		// Drain pending actions from event handlers until empty
		for actions := app.PendingActions(); len(actions) > 0; actions = app.PendingActions() {
			for _, action := range actions {
				switch act := action.(type) {
				case ext.ActionSetSessionTitle:
					if sess != nil && act.Title != "" {
						_ = sess.SetTitle(act.Title)
					}
				case ext.ActionRunAsync:
					// In single-shot mode, run async actions synchronously
					if result := act.Fn(); result != nil {
						app.EnqueueAction(result)
					}
				}
			}
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
  piglet                  Interactive TUI mode (runs setup on first launch)
  piglet <prompt>         Single-shot mode (prints response and exits)
  piglet init             Run first-time setup (config, models, API key detection)
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
  PIGLET_SMALL_MODEL      Override small model (autotitle, compaction)
`)
}
