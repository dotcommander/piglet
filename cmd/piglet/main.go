package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/internal/deploy"
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

// projectExtDir returns the project-local extensions directory if enabled, else "".
func (rt *runtime) projectExtDir() string {
	if rt.settings.AllowProjectExtensions != nil && *rt.settings.AllowProjectExtensions {
		return external.ProjectExtensionsDir(rt.cwd)
	}
	return ""
}

// writeModelsYAML writes the default model catalog. The modelsdev extension
// enriches it with live API data on first interactive startup.
func writeModelsYAML(path string) error {
	return provider.WriteDefaultModels(path)
}

func run() error {
	// Handle flags before anything else
	var debugFlag, jsonFlag bool
	var modelFlag, baseURLFlag, portFlag, resultFlag string
	var promptArgs []string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printHelp()
			return nil
		case "--version", "-v":
			fmt.Println("piglet " + resolveVersion())
			return nil
		case "--debug":
			debugFlag = true
		case "--json":
			jsonFlag = true
		case "--model":
			if i+1 >= len(args) {
				return fmt.Errorf("--model requires a value")
			}
			i++
			modelFlag = args[i]
		case "--base-url":
			if i+1 >= len(args) {
				return fmt.Errorf("--base-url requires a value")
			}
			i++
			baseURLFlag = args[i]
		case "--port":
			if i+1 >= len(args) {
				return fmt.Errorf("--port requires a value")
			}
			i++
			portFlag = args[i]
		case "--result":
			if i+1 >= len(args) {
				return fmt.Errorf("--result requires a file path")
			}
			i++
			resultFlag = args[i]
		default:
			promptArgs = append(promptArgs, args[i])
		}
	}

	// Subcommands (after flag parsing so --debug init works)
	if len(promptArgs) == 1 && promptArgs[0] == "init" {
		return config.RunSetup(writeModelsYAML, provider.SetupDefaults())
	}
	if len(promptArgs) >= 1 && (promptArgs[0] == "update" || promptArgs[0] == "upgrade") {
		settings, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		var installOpts []command.InstallOption
		for i := 1; i < len(promptArgs); i++ {
			switch promptArgs[i] {
			case "--local":
				localDir := ""
				if i+1 < len(promptArgs) && !strings.HasPrefix(promptArgs[i+1], "-") {
					i++
					localDir = promptArgs[i]
				}
				if localDir == "" {
					resolved, err := command.ResolveGoWorkExtPath()
					if err != nil {
						return fmt.Errorf("local source detection: %w", err)
					}
					localDir = resolved
				}
				installOpts = append(installOpts, command.WithLocalDir(localDir))
			default:
				return fmt.Errorf("unknown flag for update: %s", promptArgs[i])
			}
		}
		return command.RunUpdate(os.Stderr, settings, resolveVersion(), installOpts...)
	}
	if len(promptArgs) >= 1 && promptArgs[0] == "deploy" {
		var dryRun, skipSDK bool
		for i := 1; i < len(promptArgs); i++ {
			switch promptArgs[i] {
			case "--dry-run":
				dryRun = true
			case "--skip-sdk":
				skipSDK = true
			default:
				return fmt.Errorf("unknown flag for deploy: %s", promptArgs[i])
			}
		}
		return deploy.RunDeploy(os.Stderr, dryRun, skipSDK)
	}

	// Determine prompt: args after flags or stdin
	userPrompt := strings.Join(promptArgs, " ")
	interactive := userPrompt == ""

	resolvedBaseURL, err := resolveBaseURL(baseURLFlag, portFlag)
	if err != nil {
		return err
	}
	rt, debugCleanup, err := loadRuntime(debugFlag, modelFlag, resolvedBaseURL)
	if err != nil {
		return err
	}
	defer debugCleanup()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if interactive {
		return runInteractive(ctx, rt)
	}

	app, system, extCleanup := setupApp(ctx, rt, false)
	defer extCleanup()

	// If an external extension registered a provider for this model's API type, use it.
	if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
		rt.prov = p
	}

	// Session
	sess, err := session.New(rt.sessDir, rt.cwd)
	if err != nil {
		slog.Warn("session creation failed, persistence disabled", "err", err)
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

	if jsonFlag {
		if resultFlag != "" {
			fmt.Fprintf(os.Stderr, "warning: --result is ignored with --json\n")
		}
		return runJSON(ctx, ag, app, sess, userPrompt)
	}
	return runPrint(ctx, ag, app, sess, userPrompt, resultFlag)
}

// resolveModelURL detects if the model query is a URL or :port shorthand.
// Returns (baseURL, true) if it looks like a URL, ("", false) otherwise.
func resolveModelURL(query string) (string, bool) {
	if strings.HasPrefix(query, "http://") || strings.HasPrefix(query, "https://") {
		return query, true
	}
	if strings.HasPrefix(query, ":") {
		return "http://localhost" + query, true
	}
	return "", false
}

// resolveBaseURL converts the raw --base-url / --port flags into a single
// base URL string, enforcing mutual exclusion.
func resolveBaseURL(baseURL, port string) (string, error) {
	if baseURL != "" && port != "" {
		return "", fmt.Errorf("--port and --base-url are mutually exclusive")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("--port must be a valid port number (1-65535): %s", port)
		}
		return fmt.Sprintf("http://localhost:%d", n), nil
	}
	return baseURL, nil
}

// loadRuntime performs first-run setup, loads config/auth, resolves the model,
// creates the provider, and sets up debug logging.
func loadRuntime(debugFlag bool, modelOverride, baseURLOverride string) (*runtime, func(), error) {
	if config.NeedsSetup() {
		if err := config.RunSetup(writeModelsYAML, provider.SetupDefaults()); err != nil {
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

	modelQuery := modelOverride
	if modelQuery == "" {
		modelQuery = os.Getenv("PIGLET_DEFAULT_MODEL")
	}
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return nil, nil, fmt.Errorf("no default model configured\nSet defaultModel in ~/.config/piglet/config.yaml or PIGLET_DEFAULT_MODEL env var\nRun: piglet init")
	}

	// URL-as-model: if modelQuery looks like a URL, extract it and probe for model name
	if modelURL, isURL := resolveModelURL(modelQuery); isURL {
		if baseURLOverride != "" {
			return nil, nil, fmt.Errorf("cannot use both URL-as-model (%s) and --port/--base-url", modelQuery)
		}
		baseURLOverride = modelURL

		// Probe the server to discover the model ID
		result, err := provider.ProbeServer(modelURL)
		if err != nil {
			// Probe failed — use hostname as model name, continue anyway
			u, _ := url.Parse(modelURL)
			modelQuery = u.Hostname()
			if modelQuery == "" {
				modelQuery = "local"
			}
		} else {
			modelQuery = result.ModelID
			if modelQuery == "" {
				modelQuery = "local"
			}
		}
	}

	model, ok := registry.Resolve(modelQuery)
	if !ok {
		if baseURLOverride == "" {
			return nil, nil, fmt.Errorf("unknown model: %s\nUse /model to list available models, or specify a URL: piglet --model http://localhost:8080", modelQuery)
		}
		// Ad-hoc model for local server — no models.yaml entry needed
		model = core.Model{
			ID:            modelQuery,
			Name:          modelQuery,
			API:           core.APIOpenAI,
			Provider:      "local",
			BaseURL:       baseURLOverride,
			ContextWindow: config.IntOr(settings.LocalDefaults.ContextWindow, provider.LocalDefaultContextWindow()),
			MaxTokens:     config.IntOr(settings.LocalDefaults.MaxTokens, provider.LocalDefaultMaxTokens()),
		}
		registry.Register(model)
	}
	if baseURLOverride != "" {
		model.BaseURL = baseURLOverride
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
		slog.SetDefault(logger)
		if d, ok := prov.(provider.Debuggable); ok {
			d.SetLogger(logger)
		}
		logger.Info("session start", "model", model.ID, "provider", model.Provider)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, cleanup, fmt.Errorf("get cwd: %w", err)
	}

	sessDir, err := config.SessionsDir()
	if err != nil {
		slog.Warn("session directory unavailable", "err", err)
	}

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

// registerBuiltins registers all compiled-in tools, commands, and prompt sections.
func registerBuiltins(app *ext.App, rt *runtime) {
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
}

// buildSystemPrompt assembles the system prompt, resolving deferred tools note.
func buildSystemPrompt(app *ext.App, rt *runtime) string {
	deferredNote := rt.settings.DeferredToolsNote
	if deferredNote == "" {
		deferredNote = provider.DeferredToolsNote()
	}
	return prompt.Build(app, rt.settings.SystemPrompt, prompt.BuildOptions{
		OrderOverrides:    rt.settings.PromptOrder,
		DeferredToolsNote: deferredNote,
	})
}

// setupApp creates the extension app, registers tools/commands/extensions,
// builds the system prompt, and returns the app with cleanup.
func setupApp(ctx context.Context, rt *runtime, interactive bool) (*ext.App, string, func()) {
	app := ext.NewApp(rt.cwd)
	registerBuiltins(app, rt)

	loaded, extCleanup, extErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, rt.projectExtDir())
	if extErr != nil {
		slog.Warn("load extensions", "err", extErr)
	}

	if interactive && loaded == 0 {
		if err := command.InstallOfficialExtensions(os.Stderr, rt.settings); err != nil {
			fmt.Fprintf(os.Stderr, "auto-install failed: %v\n", err)
		}
		reloaded, reloadCleanup, reloadErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, rt.projectExtDir())
		if reloadErr != nil {
			slog.Warn("load extensions", "err", reloadErr)
		}
		origCleanup := extCleanup
		extCleanup = func() { reloadCleanup(); origCleanup() }
		if reloaded > 0 {
			fmt.Fprintf(os.Stderr, "Loaded %d extensions.\n\n", reloaded)
		}
	}

	prompt.RegisterSelfKnowledge(app)

	system := buildSystemPrompt(app, rt)

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
		agCfg.OnCompact = ext.CompactWithCircuitBreaker(c.Compact, 3, 5*time.Minute)
	}
	return core.NewAgent(agCfg)
}

// runInteractive starts the TUI immediately with compiled-in extensions only,
// then loads external extensions in a background goroutine via SetupFn.
func runInteractive(ctx context.Context, rt *runtime) error {
	// --- Sync phase: register compiled-in extensions only (fast, <10ms) ---
	app := ext.NewApp(rt.cwd)
	registerBuiltins(app, rt)

	// Session can be created before extensions load.
	sess, err := session.New(rt.sessDir, rt.cwd)
	if err != nil {
		slog.Warn("session creation failed, persistence disabled", "err", err)
		sess = nil
	}
	if sess != nil {
		defer sess.Close()
	}
	if sess != nil {
		if err := sess.SetModel(rt.model.ID); err != nil {
			slog.Warn("persist model to session", "error", err)
		}
	}

	// Protect extCleanup across the goroutine boundary.
	var (
		extCleanupMu sync.Mutex
		extCleanup   = func() {}
	)

	sessPtr := &sess

	setupFn := func() tui.AgentReadyMsg {
		// --- Async phase: load external extensions (~1.3s) ---
		projectDir := rt.projectExtDir()
		loaded, cleanup, loadErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, projectDir)
		if loadErr != nil {
			slog.Warn("load extensions", "err", loadErr)
		}
		extCleanupMu.Lock()
		extCleanup = cleanup
		extCleanupMu.Unlock()

		if loaded == 0 {
			if err := command.InstallOfficialExtensions(os.Stderr, rt.settings); err != nil {
				fmt.Fprintf(os.Stderr, "auto-install failed: %v\n", err)
			}
			reloaded, reloadCleanup, retryErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, projectDir)
			if retryErr != nil {
				slog.Warn("load extensions", "err", retryErr)
			}
			extCleanupMu.Lock()
			origCleanup := extCleanup
			extCleanup = func() { reloadCleanup(); origCleanup() }
			extCleanupMu.Unlock()
			if reloaded > 0 {
				fmt.Fprintf(os.Stderr, "Loaded %d extensions.\n\n", reloaded)
			}
		}

		// If an external extension registered a provider for this model's API type, use it.
		if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
			rt.prov = p
		}

		prompt.RegisterSelfKnowledge(app)

		system := buildSystemPrompt(app, rt)

		ag := buildAgent(app, rt, system)

		// Bind domain managers so commands work through ext.App.
		app.Bind(ag,
			ext.WithSessionManager(&sessionMgr{dir: rt.sessDir, current: sessPtr}),
			ext.WithModelManager(&modelMgr{registry: rt.registry, auth: rt.auth, app: app}),
		)

		return tui.AgentReadyMsg{Agent: ag}
	}

	tuiErr := tui.Run(ctx, tui.Config{
		Agent:    nil,
		Session:  sess,
		Model:    rt.model,
		Models:   rt.registry.Models(),
		SessDir:  rt.sessDir,
		Theme:    tui.DefaultTheme(),
		App:      app,
		Settings: &rt.settings,
		SetupFn:  setupFn,
	})

	extCleanupMu.Lock()
	extCleanup()
	extCleanupMu.Unlock()

	return tuiErr
}

// drainActions processes all pending ext.App actions, running async actions synchronously.
func drainActions(app *ext.App, sess *session.Session, ag ...*core.Agent) {
	for actions := app.PendingActions(); len(actions) > 0; actions = app.PendingActions() {
		for _, action := range actions {
			switch act := action.(type) {
			case ext.ActionSetSessionTitle:
				if sess != nil && act.Title != "" {
					_ = sess.SetTitle(act.Title)
				}
			case ext.ActionRunAsync:
				if result := act.Fn(); result != nil {
					app.EnqueueAction(result)
				}
			case ext.ActionShowMessage:
				fmt.Fprintln(os.Stderr, act.Text)
			case ext.ActionNotify:
				fmt.Fprintln(os.Stderr, act.Message)
			case ext.ActionSendMessage:
				if len(ag) > 0 && ag[0] != nil {
					ag[0].Steer(&core.UserMessage{Content: act.Content, Timestamp: time.Now()})
				}
			}
		}
	}
}

func runPrint(ctx context.Context, ag *core.Agent, app *ext.App, sess *session.Session, userPrompt, resultPath string) error {
	// Run message hooks for ephemeral turn context
	if injections, err := app.RunMessageHooks(ctx, userPrompt); err != nil {
		slog.Warn("message hook failed", "err", err)
	} else if len(injections) > 0 {
		ag.SetTurnContext(injections)
	}

	// Persist user message first
	if sess != nil {
		_ = sess.Append(&core.UserMessage{Content: userPrompt})
	}

	ch := ag.Start(ctx, userPrompt)

	var agentErr error
	var resultBuf strings.Builder

	for evt := range ch {
		app.DispatchEvent(ctx, evt)

		switch e := evt.(type) {
		case core.EventStreamDelta:
			if e.Kind == "text" {
				fmt.Print(e.Delta)
				if resultPath != "" {
					resultBuf.WriteString(e.Delta)
				}
			}
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
			if sess != nil {
				_ = sess.AppendCompact(ag.Messages())
			}
		case core.EventMaxTurns:
			fmt.Fprintf(os.Stderr, "[max turns reached: %d/%d]\n", e.Count, e.Max)
			agentErr = fmt.Errorf("agent stopped: max turns (%d) reached", e.Max)
		case core.EventAgentEnd:
			fmt.Println()
		}

		// Drain pending actions from event handlers until empty
		drainActions(app, sess, ag)

		if e, ok := evt.(core.EventTurnEnd); ok {
			if e.Assistant != nil {
				u := e.Assistant.Usage
				slog.Debug("turn complete",
					"input_tokens", u.InputTokens,
					"output_tokens", u.OutputTokens,
					"cache_read_tokens", u.CacheReadTokens,
					"cache_write_tokens", u.CacheWriteTokens,
				)

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

	// Write result file for tmux agent protocol (atomic: temp + rename)
	if resultPath != "" {
		dir := filepath.Dir(resultPath)
		tmp, tmpErr := os.CreateTemp(dir, ".piglet-result-*")
		if tmpErr != nil {
			slog.Warn("create temp result file", "path", resultPath, "error", tmpErr)
		} else {
			if _, writeErr := tmp.WriteString(resultBuf.String()); writeErr != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				slog.Warn("write result file", "path", resultPath, "error", writeErr)
			} else if renameErr := os.Rename(tmp.Name(), resultPath); renameErr != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				// Cross-filesystem fallback
				if copyErr := os.WriteFile(resultPath, []byte(resultBuf.String()), 0600); copyErr != nil {
					slog.Warn("write result file", "path", resultPath, "error", copyErr)
				}
			} else {
				tmp.Close()
			}
		}
	}

	return agentErr
}

func runJSON(ctx context.Context, ag *core.Agent, app *ext.App, sess *session.Session, userPrompt string) error {
	if injections, err := app.RunMessageHooks(ctx, userPrompt); err != nil {
		slog.Warn("message hook failed", "err", err)
	} else if len(injections) > 0 {
		ag.SetTurnContext(injections)
	}

	if sess != nil {
		_ = sess.Append(&core.UserMessage{Content: userPrompt})
	}

	enc := json.NewEncoder(os.Stdout)
	ch := ag.Start(ctx, userPrompt)
	var agentErr error

	for evt := range ch {
		app.DispatchEvent(ctx, evt)

		switch e := evt.(type) {
		case core.EventStreamDelta:
			_ = enc.Encode(map[string]any{"type": "stream_delta", "kind": e.Kind, "index": e.Index, "delta": e.Delta})
		case core.EventToolStart:
			_ = enc.Encode(map[string]any{"type": "tool_start", "tool": e.ToolName, "args": e.Args})
		case core.EventToolEnd:
			_ = enc.Encode(map[string]any{"type": "tool_end", "tool": e.ToolName, "is_error": e.IsError})
		case core.EventRetry:
			_ = enc.Encode(map[string]any{"type": "retry", "attempt": e.Attempt, "max": e.Max, "error": e.Error})
		case core.EventCompact:
			_ = enc.Encode(map[string]any{"type": "compact", "before": e.Before, "after": e.After, "tokens": e.TokensAtCompact})
			if sess != nil {
				_ = sess.AppendCompact(ag.Messages())
			}
		case core.EventMaxTurns:
			_ = enc.Encode(map[string]any{"type": "max_turns", "count": e.Count, "max": e.Max})
			agentErr = fmt.Errorf("agent stopped: max turns (%d) reached", e.Max)
		case core.EventAgentEnd:
			_ = enc.Encode(map[string]any{"type": "agent_end"})
		}

		drainActions(app, sess, ag)

		if e, ok := evt.(core.EventTurnEnd); ok {
			if e.Assistant != nil {
				u := e.Assistant.Usage
				_ = enc.Encode(map[string]any{
					"type": "turn_end",
					"usage": map[string]int{
						"input_tokens":       u.InputTokens,
						"output_tokens":      u.OutputTokens,
						"cache_read_tokens":  u.CacheReadTokens,
						"cache_write_tokens": u.CacheWriteTokens,
					},
				})
				if e.Assistant.StopReason == core.StopReasonError || e.Assistant.StopReason == core.StopReasonAborted {
					msg := e.Assistant.Error
					if msg == "" {
						msg = string(e.Assistant.StopReason)
					}
					agentErr = fmt.Errorf("agent error: %s", msg)
					_ = enc.Encode(map[string]any{"type": "error", "message": msg})
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
  piglet update           Upgrade piglet and rebuild extensions
  piglet update --local           Build extensions from local go.work source
  piglet update --local <path>    Build extensions from explicit local path
  piglet upgrade          Alias for update
  piglet deploy           Deploy piglet + extensions (tag, push, release)
  piglet deploy --dry-run           Show deployment plan without executing
  piglet deploy --skip-sdk          Skip SDK tag even if SDK changed
  piglet --help           Show this help
  piglet --version        Show version
  piglet --debug          Log all request/response payloads
  piglet --json           NDJSON event output (single-shot mode only)
  piglet --result <path>  Write final text output to file (for agent protocol)
  piglet --model <id>     Override model (takes precedence over env/config)
  piglet --base-url <url> Override model base URL
  piglet --port <n>       Use localhost:<n> as base URL (implies openai API)

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
  PIGLET_DEFAULT_MODEL    Override default model (--model flag takes precedence)
  PIGLET_SMALL_MODEL      Override small model (autotitle, compaction)
`)
}
