package safeguard

import "strings"

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
