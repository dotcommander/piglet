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
