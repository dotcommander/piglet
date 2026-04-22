package main

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

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
	p("  cat f.go | piglet \"review\"   Piped context with prompt\n")
	p("  echo \"hello\" | piglet        Piped input as prompt\n")
	p("  piglet init                  First-time setup (config, models, API keys)\n")
	p("  piglet update                Self-update and rebuild extensions\n\n")

	p("%s\n", heading.Render("FLAGS"))
	p("      --local            Auto-detect local model server (scans common ports)\n")
	p("      --model <query>    Model name, URL, or :port  [$PIGLET_DEFAULT_MODEL]\n")
	p("      --base-url <url>   Override provider endpoint\n")
	p("      --port <n>         Shorthand for localhost:<n> (auto-detects model)\n")
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
	p("  piglet --local                          Auto-detect local server\n")
	p("  piglet --port 11434 \"hello\"             Talk to local Ollama\n")
	p("  piglet :8080 \"summarize main.go\"        URL shorthand as model\n")
	p("  piglet --json \"list files\" | jq .       Machine-readable output\n\n")

	p("%s\n", heading.Render("INTERACTIVE"))
	p("  /help       All commands            Ctrl+C  Stop / quit\n")
	p("  /model      Switch model            Ctrl+P  Model selector\n")
	p("  /session    Switch session          Ctrl+S  Session picker\n")
	p("  /update     Self-update             Ctrl+M  Toggle mouse\n")
	p("  /mouse      Toggle mouse capture    Ctrl+Z  Suspend\n\n")

	p("%s  ~/.config/piglet/\n", heading.Render("CONFIG"))
	p("  config.yaml    Settings             auth.json      API keys\n")
	p("  prompt.md      System prompt        behavior.md    Guidelines\n")
	p("  models.yaml    Model catalog        skills/        Loaded on demand\n\n")

	p("%s\n", heading.Render("ENVIRONMENT"))
	p("  PIGLET_DEFAULT_MODEL    Override default model\n")
	p("  PIGLET_SMALL_MODEL      Override small model (autotitle, compaction)\n\n")

	p("%s  https://github.com/dotcommander/piglet\n", heading.Render("DOCS"))
}
