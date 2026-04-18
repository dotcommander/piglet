package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/ext/external"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/tool"
)

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

	// Per-tool circuit breaker: disable tools after 5 consecutive errors, re-enable after 30s.
	cbInterceptor, cbHandler := ext.NewToolCircuitBreaker(5, 30*time.Second)
	app.RegisterInterceptor(cbInterceptor)
	app.RegisterEventHandler(cbHandler)

	app.RegisterExtInfo(ext.ExtInfo{
		Name:     "builtin",
		Kind:     "builtin",
		Runtime:  "go",
		Tools:    app.Tools(),
		Commands: slices.Sorted(maps.Keys(app.Commands())),
	})
	prompt.RegisterProjectDocs(app, rt.settings.GetProjectDocs())
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

// autoInstallEnabled returns false when PIGLET_SKIP_INSTALL=1 is set,
// allowing CI or scripted callers to suppress the first-run extension install.
func autoInstallEnabled() bool {
	return os.Getenv("PIGLET_SKIP_INSTALL") == ""
}

// setupApp creates the extension app, registers tools/commands/extensions,
// builds the system prompt, and returns the app with cleanup.
func setupApp(ctx context.Context, rt *runtime) (*ext.App, string, func()) {
	app := ext.NewApp(rt.cwd)
	registerBuiltins(app, rt)

	extCleanup := loadExtensionsWithRetry(ctx, app, rt, autoInstallEnabled())

	prompt.RegisterSelfKnowledge(app)

	system := buildSystemPrompt(app, rt)

	return app, system, extCleanup
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
