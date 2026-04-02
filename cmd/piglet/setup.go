package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/command/selfupdate"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/shell"
	"github.com/dotcommander/piglet/tool"
)

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

// resolveModel determines the model from config/env/flags/URL, probing local
// servers as needed. Returns the resolved model with baseURL applied.
func resolveModel(registry *provider.Registry, settings config.Settings, modelOverride, baseURLOverride string) (core.Model, error) {
	modelQuery := modelOverride
	if modelQuery == "" {
		modelQuery = os.Getenv("PIGLET_DEFAULT_MODEL")
	}
	if modelQuery == "" {
		modelQuery = settings.DefaultModel
	}
	if modelQuery == "" {
		return core.Model{}, fmt.Errorf("no default model configured\nSet defaultModel in ~/.config/piglet/config.yaml or PIGLET_DEFAULT_MODEL env var\nRun: piglet init")
	}

	// URL-as-model: if modelQuery looks like a URL, extract it and probe for model name
	if modelURL, isURL := resolveModelURL(modelQuery); isURL {
		if baseURLOverride != "" {
			return core.Model{}, fmt.Errorf("cannot use both URL-as-model (%s) and --port/--base-url", modelQuery)
		}
		baseURLOverride = modelURL

		result, err := provider.ProbeServer(modelURL)
		if err != nil {
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
			return core.Model{}, fmt.Errorf("unknown model: %s\nUse /model to list available models, or specify a URL: piglet --model http://localhost:8080", modelQuery)
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

	return model, nil
}

// loadRuntime performs first-run setup, loads config/auth, resolves the model,
// creates the provider, and sets up debug logging.
func loadRuntime(ctx context.Context, debugFlag bool, modelOverride, baseURLOverride string) (*runtime, func(), error) {
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

	if len(settings.LocalServers) > 0 {
		ctxWin := config.IntOr(settings.LocalDefaults.ContextWindow, provider.LocalDefaultContextWindow())
		maxTok := config.IntOr(settings.LocalDefaults.MaxTokens, provider.LocalDefaultMaxTokens())
		registry.RegisterLocalServers(settings.LocalServers, ctxWin, maxTok)
	}

	if selfupdate.CheckStale() {
		go func() {
			uCtx, uCancel := context.WithTimeout(ctx, 10*time.Second)
			defer uCancel()
			if rel, err := selfupdate.FetchLatestRelease(uCtx); err == nil {
				_ = selfupdate.WriteCache(rel)
			}
		}()
	}
	if notice := selfupdate.UpdateNotice(resolveVersion()); notice != "" {
		fmt.Fprintf(os.Stderr, "%s\n", notice)
	}

	model, err := resolveModel(registry, settings, modelOverride, baseURLOverride)
	if err != nil {
		return nil, nil, err
	}

	apiKeyFn := func() string { return auth.GetAPIKey(model.Provider) }
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
	prompt.RegisterProjectDocs(app, rt.settings.GetProjectDocs())
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

// loadExtensionsWithRetry loads external extensions, auto-installs if none
// found and autoInstall is true, then reloads. Returns a composite cleanup.
func loadExtensionsWithRetry(ctx context.Context, app *ext.App, rt *runtime, autoInstall bool) func() {
	projectDir := rt.projectExtDir()
	loaded, cleanup, loadErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, projectDir)
	if loadErr != nil {
		slog.Warn("load extensions", "err", loadErr)
	}

	if autoInstall && loaded == 0 {
		if err := command.InstallOfficialExtensions(os.Stderr, rt.settings); err != nil {
			fmt.Fprintf(os.Stderr, "auto-install failed: %v\n", err)
		}
		reloaded, reloadCleanup, retryErr := external.LoadAll(ctx, app, tool.UndoSnapshots, rt.settings.DisabledExtensions, projectDir)
		if retryErr != nil {
			slog.Warn("load extensions", "err", retryErr)
		}
		origCleanup := cleanup
		cleanup = func() { reloadCleanup(); origCleanup() }
		if reloaded > 0 {
			fmt.Fprintf(os.Stderr, "Loaded %d extensions.\n\n", reloaded)
		}
	}

	return cleanup
}

// setupApp creates the extension app, registers tools/commands/extensions,
// builds the system prompt, and returns the app with cleanup.
func setupApp(ctx context.Context, rt *runtime, interactive bool) (*ext.App, string, func()) {
	app := ext.NewApp(rt.cwd)
	registerBuiltins(app, rt)

	extCleanup := loadExtensionsWithRetry(ctx, app, rt, interactive)

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

// openSession creates a session and persists the model ID. Returns nil if
// session creation fails (persistence disabled).
func openSession(rt *runtime) *session.Session {
	sess, err := session.New(rt.sessDir, rt.cwd)
	if err != nil {
		slog.Warn("session creation failed, persistence disabled", "err", err)
		return nil
	}
	if err := sess.SetModel(rt.model.ID); err != nil {
		slog.Warn("persist model to session", "error", err)
	}
	return sess
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

// shellBundle groups the objects produced by buildShell so callers can defer
// cleanup and sess.Close() correctly.
type shellBundle struct {
	sh      *shell.Shell
	sess    *session.Session
	cleanup func()
}

// buildShell runs the 6-step shell-construction sequence shared by run() and
// runREPL(): setup app → resolve provider → open session → build agent →
// create shell → attach managers.
func buildShell(ctx context.Context, rt *runtime, interactive bool) *shellBundle {
	app, system, extCleanup := setupApp(ctx, rt, interactive)

	if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
		rt.prov = p
	}

	sess := openSession(rt)
	ag := buildAgent(app, rt, system)

	sessPtr := &sess
	sh := shell.New(ctx, shell.Config{
		App:      app,
		Agent:    ag,
		Session:  sess,
		Settings: &rt.settings,
	})
	sh.SetAgent(ag,
		ext.WithSessionManager(&sessionMgr{dir: rt.sessDir, current: sessPtr}),
		ext.WithModelManager(newModelMgr(rt, app)),
	)

	return &shellBundle{
		sh:      sh,
		sess:    sess,
		cleanup: extCleanup,
	}
}
