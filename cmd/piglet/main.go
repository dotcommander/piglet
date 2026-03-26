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
	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
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

// runtime holds state threaded between startup phases.
type runtime struct {
	settings config.Settings
	auth     *config.Auth
	registry *provider.Registry
	model    core.Model
	prov     core.StreamProvider
	cwd      string
	sessDir  string
}

// writeModelsYAML writes the default model catalog. The modelsdev extension
// enriches it with live API data on first interactive startup.
func writeModelsYAML(path string) error {
	return provider.WriteDefaultModels(path)
}

func run() error {
	// Handle flags before anything else
	var debugFlag bool
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
			debugFlag = true
		default:
			promptArgs = append(promptArgs, arg)
		}
	}

	// Subcommands (after flag parsing so --debug init works)
	if len(promptArgs) == 1 && promptArgs[0] == "init" {
		return config.RunSetup(writeModelsYAML)
	}
	if len(promptArgs) == 1 && promptArgs[0] == "update" {
		settings, _ := config.Load()
		return command.RunUpdate(os.Stderr, settings)
	}
	if len(promptArgs) == 1 && promptArgs[0] == "upgrade" {
		return command.RunUpgrade(os.Stderr, resolveVersion())
	}

	// Determine prompt: args after flags or stdin
	userPrompt := strings.Join(promptArgs, " ")
	interactive := userPrompt == ""

	rt, debugCleanup, err := loadRuntime(debugFlag)
	if err != nil {
		return err
	}
	defer debugCleanup()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app, system, extCleanup := setupApp(ctx, rt, interactive)
	defer extCleanup()

	// If an external extension registered a provider for this model's API type, use it.
	if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
		rt.prov = p
	}

	// Session
	sess, err := session.New(rt.sessDir, rt.cwd)
	if err != nil {
		sess = nil
	}
	if sess != nil {
		defer sess.Close()
		if err := sess.SetModel(rt.model.ID); err != nil {
			slog.Warn("persist model to session", "error", err)
		}
	}

	ag := buildAgent(app, rt, system)

	// Bind domain managers so commands work through ext.App
	sessPtr := &sess
	app.Bind(ag,
		ext.WithSessionManager(&sessionMgr{dir: rt.sessDir, current: sessPtr}),
		ext.WithModelManager(&modelMgr{registry: rt.registry, auth: rt.auth, app: app}),
	)

	if interactive {
		return tui.Run(tui.Config{
			Agent:    ag,
			Session:  sess,
			Model:    rt.model,
			Models:   rt.registry.Models(),
			SessDir:  rt.sessDir,
			Theme:    tui.DefaultTheme(),
			App:      app,
			Settings: &rt.settings,
		})
	}

	return runPrint(ag, app, sess, userPrompt)
}

// loadRuntime performs first-run setup, loads config/auth, resolves the model,
// creates the provider, and sets up debug logging.
func loadRuntime(debugFlag bool) (*runtime, func(), error) {
	if config.NeedsSetup() {
		if err := config.RunSetup(writeModelsYAML); err != nil {
			return nil, nil, fmt.Errorf("setup: %w", err)
		}
	}

	settings, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}

	auth, err := config.NewAuthDefault()
	if err != nil {
		return nil, nil, fmt.Errorf("init auth: %w", err)
	}

	registry := provider.NewRegistry()

	if selfupdate.CheckStale() {
		go func() {
			uCtx, uCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer uCancel()
			if rel, err := selfupdate.FetchLatestRelease(uCtx); err == nil {
				_ = selfupdate.WriteCache(rel)
			}
		}()
	}
	if notice := selfupdate.UpdateNotice(resolveVersion()); notice != "" {
		fmt.Fprintf(os.Stderr, "%s\n", notice)
	}

	modelQuery := os.Getenv("PIGLET_DEFAULT_MODEL")
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return nil, nil, fmt.Errorf("no default model configured\nSet defaultModel in ~/.config/piglet/config.yaml or PIGLET_DEFAULT_MODEL env var\nRun: piglet init")
	}

	model, ok := registry.Resolve(modelQuery)
	if !ok {
		return nil, nil, fmt.Errorf("unknown model: %s", modelQuery)
	}

	apiKeyFn := func() string {
		return auth.GetAPIKey(model.Provider)
	}
	prov, err := registry.Create(model, apiKeyFn)
	if err != nil {
		return nil, nil, fmt.Errorf("create provider: %w", err)
	}

	cleanup := func() {}
	if debugFlag || settings.Debug {
		logger, logCleanup, err := openDebugLog()
		if err != nil {
			return nil, nil, fmt.Errorf("debug log: %w", err)
		}
		cleanup = logCleanup
		if d, ok := prov.(provider.Debuggable); ok {
			d.SetLogger(logger)
		}
		logger.Info("session start", "model", model.ID, "provider", model.Provider)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, cleanup, fmt.Errorf("get cwd: %w", err)
	}

	sessDir, _ := config.SessionsDir()

	rt := &runtime{
		settings: settings,
		auth:     auth,
		registry: registry,
		model:    model,
		prov:     prov,
		cwd:      cwd,
		sessDir:  sessDir,
	}
	return rt, cleanup, nil
}

// setupApp creates the extension app, registers tools/commands/extensions,
// builds the system prompt, and returns the app with cleanup.
func setupApp(ctx context.Context, rt *runtime, interactive bool) (*ext.App, string, func()) {
	app := ext.NewApp(rt.cwd)
	tool.RegisterBuiltins(app, tool.BashConfig{
		DefaultTimeout: rt.settings.Bash.DefaultTimeout,
		MaxTimeout:     rt.settings.Bash.MaxTimeout,
		MaxStdout:      rt.settings.Bash.MaxStdout,
		MaxStderr:      rt.settings.Bash.MaxStderr,
	}, tool.ToolConfig{
		ReadLimit: rt.settings.Tools.ReadLimit,
		GrepLimit: rt.settings.Tools.GrepLimit,
	})
	command.RegisterBuiltins(app, rt.settings.Shortcuts, resolveVersion())
	app.RegisterExtInfo(ext.ExtInfo{
		Name:     "builtin",
		Kind:     "builtin",
		Runtime:  "go",
		Tools:    app.Tools(),
		Commands: slices.Sorted(maps.Keys(app.Commands())),
	})

	prompt.RegisterProjectDocs(app, rt.settings.ProjectDocs)

	loaded, extCleanup, _ := external.LoadAll(ctx, app, tool.UndoSnapshots)

	if interactive && loaded == 0 {
		if err := command.InstallOfficialExtensions(os.Stderr, rt.settings); err != nil {
			fmt.Fprintf(os.Stderr, "auto-install failed: %v\n", err)
		}
		reloaded, reloadCleanup, _ := external.LoadAll(ctx, app, tool.UndoSnapshots)
		origCleanup := extCleanup
		extCleanup = func() { reloadCleanup(); origCleanup() }
		if reloaded > 0 {
			fmt.Fprintf(os.Stderr, "Loaded %d extensions.\n\n", reloaded)
		}
	}

	prompt.RegisterSelfKnowledge(app)

	basePrompt := rt.settings.SystemPrompt
	system := prompt.Build(app, basePrompt, prompt.BuildOptions{
		OrderOverrides: rt.settings.PromptOrder,
	})

	return app, system, extCleanup
}

// buildAgent creates the agent from app state and runtime config.
func buildAgent(app *ext.App, rt *runtime, system string) *core.Agent {
	coreTools := app.CoreTools()
	var opts core.StreamOptions
	if rt.settings.Agent.MaxTokens > 0 {
		mt := rt.settings.Agent.MaxTokens
		opts.MaxTokens = &mt
	}
	agCfg := core.AgentConfig{
		Provider:        rt.prov,
		System:          system,
		Tools:           coreTools,
		Options:         opts,
		MaxTurns:        config.IntOr(rt.settings.Agent.MaxTurns, 10),
		MaxMessages:     config.IntOr(rt.settings.Agent.MaxMessages, 200),
		MaxRetries:      rt.settings.Agent.MaxRetries,
		ToolConcurrency: rt.settings.Agent.ToolConcurrency,
	}
	if c := app.Compactor(); c != nil {
		agCfg.CompactAt = c.Threshold
		agCfg.OnCompact = c.Compact
	}
	return core.NewAgent(agCfg)
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
	return logger, func() { _ = f.Close() }, nil
}

func printHelp() {
	fmt.Print(`piglet — minimalist coding assistant

Usage:
  piglet                  Interactive TUI mode (runs setup on first launch)
  piglet <prompt>         Single-shot mode (prints response and exits)
  piglet init             Run first-time setup (config, models, API key detection)
  piglet update           Update extensions to latest
  piglet upgrade          Upgrade piglet to latest release
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
