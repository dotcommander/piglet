package safeguard

import "strings"

// CommandKind classifies a bash command's mutation intent.
type CommandKind int

const (
	// CommandUnknown means the classifier cannot determine intent.
	CommandUnknown CommandKind = iota
	// CommandReadOnly means the command only reads state — no file/system mutation.
	CommandReadOnly
	// CommandWrite means the command mutates files, processes, or system state.
	CommandWrite
)

func (k CommandKind) String() string {
	switch k {
	case CommandReadOnly:
		return "read_only"
	case CommandWrite:
		return "write"
	default:
		return "unknown"
	}
}

// ClassifyCommand determines whether a bash command is read-only, write, or unknown.
// Conservative: returns ReadOnly only when every segment is a known safe command
// with no suspicious metacharacters.
func ClassifyCommand(cmd string) CommandKind {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return CommandUnknown
	}

	// Commands with command substitution or backticks are never simple reads.
	stripped := stripSingleQuotes(cmd)
	if strings.Contains(stripped, "$(") || strings.Contains(stripped, "`") {
		return CommandUnknown
	}

	// Output redirects (>, >>) make any command a write.
	if hasOutputRedirect(stripped) {
		return CommandWrite
	}

	// Split on chain operators (;, &&, ||) and pipes (|), classify each segment.
	segments := splitSegments(cmd)
	if len(segments) == 0 {
		return CommandUnknown
	}

	allRead := true
	for _, seg := range segments {
		kind := classifySegment(strings.TrimSpace(seg))
		if kind == CommandWrite {
			return CommandWrite
		}
		if kind != CommandReadOnly {
			allRead = false
		}
	}

	if allRead {
		return CommandReadOnly
	}
	return CommandUnknown
}

// classifySegment classifies a single command (no pipes or chain operators).
func classifySegment(seg string) CommandKind {
	base := extractBaseCommand(seg)
	if base == "" {
		return CommandUnknown
	}

	// Check pure read/write sets first.
	if readOnlyCommands[base] {
		return CommandReadOnly
	}
	if writeCommands[base] {
		return CommandWrite
	}

	// Mixed commands: classify by subcommand or flags.
	if fn, ok := mixedClassifiers[base]; ok {
		return fn(seg)
	}

	return CommandUnknown
}

// extractBaseCommand returns the first command word, skipping env var assignments.
func extractBaseCommand(seg string) string {
	seg = strings.TrimSpace(seg)
	// Strip leading subshell parens.
	seg = strings.TrimLeft(seg, "(")
	seg = strings.TrimSpace(seg)

	// Skip env var assignments: FOO=bar cmd
	for {
		word := firstWord(seg)
		if word == "" {
			return ""
		}
		if strings.Contains(word, "=") && !strings.HasPrefix(word, "=") {
			after := strings.TrimSpace(seg[len(word):])
			if after == "" {
				return "" // bare assignment
			}
			seg = after
			continue
		}
		break
	}
	return firstWord(seg)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i]
		}
	}
	return s
}

// splitSegments splits a command on ;, &&, ||, and | outside quotes.
func splitSegments(cmd string) []string {
	var segments []string
	var current strings.Builder
	runes := []rune(cmd)
	inSingle, inDouble := false, false

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			current.WriteRune(r)
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			current.WriteRune(r)
			continue
		}
		if inSingle || inDouble {
			current.WriteRune(r)
			continue
		}

		switch {
		case r == ';':
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
		case r == '&' && i+1 < len(runes) && runes[i+1] == '&':
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
			i++ // skip second &
		case r == '|' && i+1 < len(runes) && runes[i+1] == '|':
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
			i++ // skip second |
		case r == '|':
			if s := strings.TrimSpace(current.String()); s != "" {
				segments = append(segments, s)
			}
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		segments = append(segments, s)
	}
	return segments
}

// hasOutputRedirect detects >, >> outside quotes (excluding 2> stderr and >& fd dups).
func hasOutputRedirect(stripped string) bool {
	runes := []rune(stripped)
	for i, r := range runes {
		if r != '>' {
			continue
		}
		// 2> or 2>> (stderr redirect) — skip.
		if i > 0 && runes[i-1] == '2' {
			continue
		}
		// >& (fd duplicate like >&2) — skip.
		if i+1 < len(runes) && runes[i+1] == '&' {
			continue
		}
		// Second > in >> already counted by first — skip.
		if i > 0 && runes[i-1] == '>' {
			continue
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Command databases
// ---------------------------------------------------------------------------

var readOnlyCommands = map[string]bool{
	// File inspection
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"file": true, "stat": true, "wc": true,
	// Directory listing
	"ls": true, "tree": true, "du": true, "df": true, "pwd": true,
	// Search
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true,
	"which": true, "whereis": true, "type": true,
	// Text processing (read-only, no -i)
	"sort": true, "uniq": true, "cut": true, "tr": true, "diff": true,
	"jq": true, "yq": true, "column": true, "rev": true, "tac": true,
	// System/env info
	"echo": true, "printf": true, "date": true, "whoami": true,
	"hostname": true, "uname": true, "id": true, "groups": true,
	"uptime": true, "nproc": true, "arch": true,
	"env": true, "printenv": true,
	// Path utilities
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	// Checksums
	"md5sum": true, "sha256sum": true, "sha1sum": true,
	// Process info
	"ps": true, "pgrep": true,
	// Navigation (ephemeral in subprocess)
	"cd": true, "pushd": true, "popd": true,
	// Flow control / no-ops
	"true": true, "false": true, "test": true, "[": true, "exit": true,
}

var writeCommands = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true,
	"chmod": true, "chown": true, "chgrp": true,
	"mkdir": true, "touch": true, "ln": true,
	"dd": true, "mkfs": true, "tee": true,
	"kill": true, "pkill": true, "killall": true,
	"reboot": true, "shutdown": true, "halt": true,
}

// ---------------------------------------------------------------------------
// Mixed command classifiers
// ---------------------------------------------------------------------------

var mixedClassifiers = map[string]func(string) CommandKind{
	"git":    classifyGit,
	"find":   classifyFind,
	"sed":    classifySed,
	"go":     classifyGo,
	"docker": classifyDocker,
	"podman": classifyDocker,
}

var gitReadSubcmds = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"describe": true, "rev-parse": true, "rev-list": true,
	"ls-files": true, "ls-tree": true, "ls-remote": true,
	"cat-file": true, "shortlog": true, "blame": true,
	"version": true, "help": true, "remote": true,
	"config": true, // reading config; --set handled by flag check
}

var gitWriteSubcmds = map[string]bool{
	"push": true, "pull": true, "fetch": true,
	"commit": true, "merge": true, "rebase": true,
	"reset": true, "checkout": true, "switch": true,
	"cherry-pick": true, "revert": true, "am": true,
	"clean": true, "gc": true, "prune": true,
	"init": true, "clone": true, "stash": true,
	"add": true, "rm": true, "mv": true, "restore": true,
	"tag": true, "branch": true, // can delete; conservative
}

func classifyGit(seg string) CommandKind {
	sub := subcommand(seg, "git")
	if sub == "" {
		return CommandUnknown
	}
	if gitWriteSubcmds[sub] {
		return CommandWrite
	}
	if gitReadSubcmds[sub] {
		return CommandReadOnly
	}
	return CommandUnknown
}

func classifyFind(seg string) CommandKind {
	lower := strings.ToLower(seg)
	for _, flag := range []string{"-exec", "-execdir", "-delete", "-ok"} {
		if strings.Contains(lower, flag) {
			return CommandWrite
		}
	}
	return CommandReadOnly
}

func classifySed(seg string) CommandKind {
	// sed -i (in-place edit) is a write; without -i it's read-only.
	if hasFlag(seg, "-i") || strings.Contains(seg, "--in-place") {
		return CommandWrite
	}
	return CommandReadOnly
}

var goReadSubcmds = map[string]bool{
	"version": true, "env": true, "list": true, "doc": true,
	"help": true, "tool": true,
}

func classifyGo(seg string) CommandKind {
	sub := subcommand(seg, "go")
	if goReadSubcmds[sub] {
		return CommandReadOnly
	}
	return CommandUnknown
}

var dockerReadSubcmds = map[string]bool{
	"ps": true, "images": true, "inspect": true, "logs": true,
	"version": true, "info": true, "top": true, "stats": true,
	"port": true, "history": true, "search": true,
}

func classifyDocker(seg string) CommandKind {
	// Detect base command (docker or podman).
	base := extractBaseCommand(seg)
	sub := subcommand(seg, base)
	if dockerReadSubcmds[sub] {
		return CommandReadOnly
	}
	return CommandUnknown
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// subcommand extracts the first non-flag argument after the base command.
func subcommand(seg, base string) string {
	after := strings.TrimSpace(seg[strings.Index(seg, base)+len(base):])
	for _, word := range strings.Fields(after) {
		if strings.HasPrefix(word, "-") {
			continue // skip flags
		}
		return word
	}
	return ""
}

// hasFlag checks if a short flag appears in the command arguments.
func hasFlag(seg, flag string) bool {
	for _, word := range strings.Fields(seg) {
		if word == flag {
			return true
		}
		// Handle combined short flags: -inE contains -i
		if len(flag) == 2 && flag[0] == '-' && len(word) > 1 && word[0] == '-' && word[1] != '-' {
			if strings.ContainsRune(word[1:], rune(flag[1])) {
				return true
			}
		}
	}
	return false
}
