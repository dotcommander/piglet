package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"

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

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	debug      bool
	json       bool
	repl       bool
	local      bool
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
		case "--local":
			f.local = true
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

// handleSubcommands processes init/update subcommands.
// Returns (true, nil) if a subcommand was handled, (false, nil) to continue.
func handleSubcommands(args []string) (bool, error) {
	if len(args) == 1 && args[0] == "init" {
		return true, config.RunSetup(writeModelsYAML, provider.SetupDefaults())
	}
	if len(args) >= 1 && (args[0] == "update" || args[0] == "upgrade") {
		settings, err := config.Load()
		if err != nil {
			return true, fmt.Errorf("config: %w", err)
		}
		// --extensions-only: called by the old binary after self-upgrade
		// so the NEW binary's code handles extension installation.
		if len(args) >= 2 && args[1] == "--extensions-only" {
			return true, command.InstallOfficialExtensions(os.Stderr, settings)
		}
		return true, command.RunUpdate(os.Stderr, settings, resolveVersion())
	}
	return false, nil
}

// readStdin reads piped stdin and combines it with positional prompt args.
func readStdin(promptArgs []string) (userPrompt string, stdinPiped bool, err error) {
	userPrompt = strings.Join(promptArgs, " ")
	info, statErr := os.Stdin.Stat()
	if statErr != nil {
		return userPrompt, false, nil
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		stdinPiped = true
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return userPrompt, stdinPiped, readErr
		}
		if piped := strings.TrimSpace(string(data)); piped != "" {
			if userPrompt == "" {
				userPrompt = piped
			} else {
				userPrompt = userPrompt + "\n\n" + piped
			}
		}
	}
	return userPrompt, stdinPiped, nil
}

// resolveLocalModel handles --local scanning and auto-probe for local servers.
func resolveLocalModel(flags cliFlags, baseURL string) (resolvedBaseURL, model string, _ error) {
	if flags.local {
		if baseURL != "" {
			return "", "", fmt.Errorf("--local and --port/--base-url are mutually exclusive")
		}
		scan, err := provider.ScanLocalServers()
		if err != nil {
			return "", "", err
		}
		baseURL = scan.URL
		// Reuse the models already fetched during the scan — no second probe needed.
		if flags.model == "" {
			if m := provider.BestModel(scan.Models); m != "" {
				flags.model = m
			}
		}
	}

	// Auto-probe model when connecting to a local server without explicit --model.
	if flags.model == "" && baseURL != "" {
		result, err := provider.ProbeServer(baseURL)
		if err != nil {
			return "", "", fmt.Errorf("could not detect model at %s: %w\nSpecify with --model <name>", baseURL, err)
		}
		flags.model = result.ModelID
		if flags.model == "" {
			flags.model = "default"
		}
	}
	return baseURL, flags.model, nil
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

	if handled, err := handleSubcommands(flags.promptArgs); handled || err != nil {
		return err
	}

	userPrompt, stdinPiped, err := readStdin(flags.promptArgs)
	if err != nil {
		return err
	}

	interactive := userPrompt == ""
	if interactive && stdinPiped {
		return fmt.Errorf("no input: stdin pipe was empty and no prompt given")
	}

	baseURL, err := resolveBaseURL(flags.baseURL, flags.port)
	if err != nil {
		return err
	}

	baseURL, flags.model, err = resolveLocalModel(flags, baseURL)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rt, debugCleanup, err := loadRuntime(ctx, flags.debug, flags.model, baseURL)
	if err != nil {
		return err
	}
	defer debugCleanup()

	if interactive {
		// Redirect slog to file so extension-init warnings and other log calls
		// do not corrupt the bubbletea TUI frames. Skip when --debug is set
		// because loadRuntime already installed the debug file handler.
		if !flags.debug && !rt.settings.Debug {
			tuiLogCleanup := setupTUILogging()
			defer tuiLogCleanup()
		}
		if flags.repl {
			return runREPL(ctx, rt)
		}
		return runInteractive(ctx, rt)
	}

	b := buildShell(ctx, rt, false)
	defer b.cleanup()
	if b.sess != nil {
		defer b.sess.Close()
	}

	if flags.json {
		if flags.result != "" {
			fmt.Fprintf(os.Stderr, "warning: --result is ignored with --json\n")
		}
		return runJSON(ctx, b.sh, userPrompt)
	}
	return runPrint(ctx, b.sh, userPrompt, flags.result)
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

		attachManagers(sh, ag, app, rt, sessPtr)

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
