package main

import (
	"context"
	"time"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/session"
	"github.com/dotcommander/piglet/shell"
)

// buildSystemPrompt assembles the system prompt with tool mode applied.
func buildSystemPrompt(app *ext.App, rt *runtime) string {
	return prompt.Build(app, rt.settings.SystemPrompt, &prompt.BuildOptions{
		OrderOverrides: rt.settings.PromptOrder,
		ToolMode:       toolModeForModel(rt.model),
	})
}

// toolModeForModel returns the tool disclosure mode for the given model.
// Local providers get compact mode (deferred tools stripped); cloud gets full.
func toolModeForModel(m core.Model) ext.ToolMode {
	if provider.IsLocalProvider(m.Provider) {
		return ext.ToolModeCompact
	}
	return ext.ToolModeFull
}

// buildAgent creates the agent from app state and runtime config.
func buildAgent(app *ext.App, rt *runtime, system string) *core.Agent {
	mode := toolModeForModel(rt.model)
	coreTools := app.CoreToolsForModel(mode)
	app.SetToolMode(mode)
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
		MaxTurns:        config.IntOr(rt.settings.Agent.MaxTurns, config.DefaultMaxTurns),
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

// shellBundle groups the objects produced by buildShell so callers can defer
// cleanup and sess.Close() correctly.
type shellBundle struct {
	sh      *shell.Shell
	sess    *session.Session
	cleanup func()
}

// buildShell runs the shell-construction sequence shared by run() and runREPL():
// setup app → resolve provider → open session → build agent → create shell → attach managers.
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
	attachManagers(sh, ag, app, rt, sessPtr)

	return &shellBundle{
		sh:      sh,
		sess:    sess,
		cleanup: extCleanup,
	}
}

// attachManagers wires the session manager, model manager, and tool activator
// onto the shell. Shared by buildShell and runInteractive to ensure consistent wiring.
func attachManagers(sh *shell.Shell, ag *core.Agent, app *ext.App, rt *runtime, sessPtr **session.Session) {
	sh.SetAgent(ag,
		ext.WithSessionManager(&sessionMgr{dir: rt.sessDir, current: sessPtr}),
		ext.WithModelManager(newModelMgr(rt, app)),
		ext.WithToolActivator(func() {
			ag.SetTools(app.CoreTools())
		}),
	)
}
