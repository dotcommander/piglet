package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"

	"charm.land/lipgloss/v2"
	"github.com/dotcommander/piglet/command"
	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/prompt"
	"github.com/dotcommander/piglet/provider"
	"github.com/dotcommander/piglet/shell"
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
	flags, err := parseFlags(os.Args[1:])
	if err != nil {
		return err
	}
	if flags.help {
		printHelp()
		return nil
	}
	if flags.version {
		fmt.Println("piglet " + resolveVersion())
		return nil
	}

	// Subcommands
	if len(flags.promptArgs) == 1 && flags.promptArgs[0] == "init" {
		return config.RunSetup(writeModelsYAML, provider.SetupDefaults())
	}
	if len(flags.promptArgs) >= 1 && (flags.promptArgs[0] == "update" || flags.promptArgs[0] == "upgrade") {
		settings, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		return command.RunUpdate(os.Stderr, settings, resolveVersion())
	}

	userPrompt := strings.Join(flags.promptArgs, " ")
	interactive := userPrompt == ""

	resolvedBaseURL, err := resolveBaseURL(flags.baseURL, flags.port)
	if err != nil {
		return err
	}
	rt, debugCleanup, err := loadRuntime(flags.debug, flags.model, resolvedBaseURL)
	if err != nil {
		return err
	}
	defer debugCleanup()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if interactive {
		if flags.repl {
			return runREPL(ctx, rt)
		}
		return runInteractive(ctx, rt)
	}

	app, system, extCleanup := setupApp(ctx, rt, false)
	defer extCleanup()

	if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
		rt.prov = p
	}

	sess := openSession(rt)
	if sess != nil {
		defer sess.Close()
	}

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

	if flags.json {
		if flags.result != "" {
			fmt.Fprintf(os.Stderr, "warning: --result is ignored with --json\n")
		}
		return runJSON(ctx, sh, userPrompt)
	}
	return runPrint(ctx, sh, userPrompt, flags.result)
}

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	debug      bool
	json       bool
	repl       bool
	help       bool
	version    bool
	model      string
	baseURL    string
	port       string
	result     string
	promptArgs []string
}

func parseFlags(args []string) (cliFlags, error) {
	var f cliFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			f.help = true
			return f, nil
		case "--version", "-v":
			f.version = true
			return f, nil
		case "--debug":
			f.debug = true
		case "--json":
			f.json = true
		case "--repl":
			f.repl = true
		case "--model":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--model requires a value")
			}
			i++
			f.model = args[i]
		case "--base-url":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--base-url requires a value")
			}
			i++
			f.baseURL = args[i]
		case "--port":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--port requires a value")
			}
			i++
			f.port = args[i]
		case "--result":
			if i+1 >= len(args) {
				return f, fmt.Errorf("--result requires a file path")
			}
			i++
			f.result = args[i]
		default:
			f.promptArgs = append(f.promptArgs, args[i])
		}
	}
	return f, nil
}

// runInteractive starts the TUI immediately with compiled-in extensions only,
// then loads external extensions in a background goroutine via SetupFn.
func runInteractive(ctx context.Context, rt *runtime) error {
	app := ext.NewApp(rt.cwd)
	registerBuiltins(app, rt)

	sess := openSession(rt)
	if sess != nil {
		defer sess.Close()
	}

	var (
		extCleanupMu sync.Mutex
		extCleanup   = func() {}
	)

	sessPtr := &sess
	sh := shell.New(ctx, shell.Config{
		App:      app,
		Agent:    nil,
		Session:  sess,
		Settings: &rt.settings,
	})

	setupFn := func() tui.AgentReadyMsg {
		cleanup := loadExtensionsWithRetry(ctx, app, rt, true)
		extCleanupMu.Lock()
		extCleanup = cleanup
		extCleanupMu.Unlock()

		if p, ok := app.StreamProvider(string(rt.model.API), rt.model); ok {
			rt.prov = p
		}

		prompt.RegisterSelfKnowledge(app)
		system := buildSystemPrompt(app, rt)
		ag := buildAgent(app, rt, system)

		sh.SetAgent(ag,
			ext.WithSessionManager(&sessionMgr{dir: rt.sessDir, current: sessPtr}),
			ext.WithModelManager(newModelMgr(rt, app)),
		)

		return tui.AgentReadyMsg{Agent: ag}
	}

	tuiErr := tui.Run(ctx, tui.Config{
		Shell:    sh,
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

func printHelp() {
	w := lipgloss.Writer

	brand := lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	heading := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	p := func(f string, a ...any) { fmt.Fprintf(w, f, a...) }

	p("%s %s — extension-first coding assistant\n\n", brand.Render("piglet"), dim.Render(resolveVersion()))

	p("%s\n", heading.Render("USAGE"))
	p("  piglet [flags] [prompt]\n\n")

	p("%s\n", heading.Render("MODES"))
	p("  piglet                       Interactive TUI session\n")
	p("  piglet \"fix the tests\"       Single-shot — print response and exit\n")
	p("  piglet init                  First-time setup (config, models, API keys)\n")
	p("  piglet update                Self-update and rebuild extensions\n\n")

	p("%s\n", heading.Render("FLAGS"))
	p("      --model <query>    Model name, URL, or :port  [$PIGLET_DEFAULT_MODEL]\n")
	p("      --base-url <url>   Override provider endpoint\n")
	p("      --port <n>         Shorthand for localhost:<n>\n")
	p("      --repl             Simple REPL mode (no TUI)\n")
	p("      --json             NDJSON event stream (single-shot only)\n")
	p("      --result <path>    Write final output to file\n")
	p("      --debug            Log payloads to debug.log\n")
	p("  -v, --version          Print version\n")
	p("  -h, --help             Print this help\n\n")

	p("%s\n", heading.Render("EXAMPLES"))
	p("  piglet                                  Start interactive session\n")
	p("  piglet \"what does this project do?\"     Quick question\n")
	p("  piglet --model sonnet \"hello\"           Use a specific model\n")
	p("  piglet --port 11434 \"hello\"             Talk to local Ollama\n")
	p("  piglet :8080 \"summarize main.go\"        URL shorthand as model\n")
	p("  piglet --json \"list files\" | jq .       Machine-readable output\n\n")

	p("%s\n", heading.Render("INTERACTIVE"))
	p("  /help       All commands            Ctrl+C  Stop / quit\n")
	p("  /model      Switch model            Ctrl+P  Model selector\n")
	p("  /session    Switch session          Ctrl+S  Session picker\n")
	p("  /compact    Compress history        Ctrl+M  Toggle mouse\n")
	p("  /clear      Reset conversation      Ctrl+Z  Suspend\n\n")

	p("%s  ~/.config/piglet/\n", heading.Render("CONFIG"))
	p("  config.yaml    Settings             auth.json      API keys\n")
	p("  prompt.md      System prompt        behavior.md    Guidelines\n")
	p("  models.yaml    Model catalog        skills/        Loaded on demand\n\n")

	p("%s\n", heading.Render("ENVIRONMENT"))
	p("  PIGLET_DEFAULT_MODEL    Override default model\n")
	p("  PIGLET_SMALL_MODEL      Override small model (autotitle, compaction)\n\n")

	p("%s  https://github.com/dotcommander/piglet\n", heading.Render("DOCS"))
}
