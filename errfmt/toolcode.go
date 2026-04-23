package errfmt

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// ToolErrorCode is a stable machine-readable identifier for tool-execution
// failures. Consumers (LLMs via prefix matching, interceptors via ParseToolError)
// can branch on these codes to decide whether to retry, reword, or abort.
//
// Extensions may define their own codes — pass any uppercase snake_case string
// to ToolErr. The core set below covers the compiled-in tools.
type ToolErrorCode string

const (
	// ToolErrInvalidArgs — required argument missing, wrong type, or empty.
	// Recovery: fix arguments and retry.
	ToolErrInvalidArgs ToolErrorCode = "INVALID_ARGS"

	// ToolErrFileNotFound — path does not exist, or stat failed with ENOENT.
	// Recovery: verify path; consider running `find`/`grep` first.
	ToolErrFileNotFound ToolErrorCode = "FILE_NOT_FOUND"

	// ToolErrFileStale — TOCTOU mtime mismatch between last read and current write.
	// Recovery: re-read the file before writing.
	ToolErrFileStale ToolErrorCode = "FILE_STALE"

	// ToolErrFileTooLarge — file exceeds the tool's size limit.
	// Recovery: use offset/limit, or process in chunks.
	ToolErrFileTooLarge ToolErrorCode = "FILE_TOO_LARGE"

	// ToolErrNotRegularFile — target is a directory, symlink loop, device, etc.
	// Recovery: target a regular file.
	ToolErrNotRegularFile ToolErrorCode = "NOT_REGULAR_FILE"

	// ToolErrPermissionDenied — OS permission error (EACCES).
	// Recovery: cannot be resolved from within the tool — surface to user.
	ToolErrPermissionDenied ToolErrorCode = "PERMISSION_DENIED"

	// ToolErrNotUnique — edit's old_text matched zero or multiple locations.
	// Recovery: add surrounding context to make the match unique.
	ToolErrNotUnique ToolErrorCode = "NOT_UNIQUE"

	// ToolErrTimeout — operation exceeded its deadline (e.g. bash).
	// Recovery: increase timeout or run a narrower command.
	ToolErrTimeout ToolErrorCode = "TIMEOUT"

	// ToolErrExitNonzero — subprocess finished with non-zero exit code.
	// Recovery: inspect stderr; may be expected (grep: no matches).
	ToolErrExitNonzero ToolErrorCode = "EXIT_NONZERO"

	// ToolErrIO — unclassified I/O error (read, write, mkdir).
	// Recovery: inspect message; often transient.
	ToolErrIO ToolErrorCode = "IO_ERROR"

	// ToolErrInternal — bug in the tool itself (panic recovered, invariant broken).
	// Recovery: report; not user-actionable.
	ToolErrInternal ToolErrorCode = "INTERNAL"

	// ToolErrInterrupted — tool execution did not complete because the session was
	// interrupted (crash, kill -9) before the result was recorded.
	// Recovery: the agent will synthesize a placeholder result; retry if needed.
	ToolErrInterrupted ToolErrorCode = "TOOL_INTERRUPTED"

	// ToolErrToolDisabled — tool has been disabled by the circuit breaker after
	// too many consecutive errors.
	// Recovery: fix the tool invocation or /clear to reset the session breaker state.
	ToolErrToolDisabled ToolErrorCode = "TOOL_DISABLED"
)

// ToolErr builds a *core.ToolResult representing a coded tool error.
// Format (exact):
//
//	[error:CODE] <summary>
//	hint: <hint>        (omitted if hint is empty)
//
// code must be uppercase snake_case (extensions may define custom codes).
// summary must be non-empty, one line, no trailing period.
// hint may be empty; when non-empty, it should be an actionable sentence.
func ToolErr(code ToolErrorCode, summary, hint string) *core.ToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "[error:%s] %s", code, summary)
	if hint != "" {
		b.WriteString("\nhint: ")
		b.WriteString(hint)
	}
	return &core.ToolResult{
		Content: []core.ContentBlock{core.TextContent{Text: b.String()}},
	}
}

// ParsedToolError holds the fields extracted from a coded tool-error text.
type ParsedToolError struct {
	Code    ToolErrorCode
	Summary string
	Hint    string
	Body    string // anything after the header+hint (empty for most cases)
}

// ParseToolError extracts the code/summary/hint from text produced by ToolErr.
// Returns nil if text does not start with the "[error:" prefix.
// Tolerates a trailing body after the hint (or after the header if hint is
// absent) separated by a blank line.
func ParseToolError(text string) *ParsedToolError {
	if !strings.HasPrefix(text, "[error:") {
		return nil
	}

	// Find closing "]" of the code bracket.
	bracket := strings.Index(text, "]")
	if bracket < 0 {
		return nil
	}

	code := ToolErrorCode(text[len("[error:"):bracket])

	// Everything after "] " is the summary line (up to the first newline).
	rest := text[bracket+1:]
	if strings.HasPrefix(rest, " ") {
		rest = rest[1:]
	}

	var summary, hint, body string
	// Split into lines to find summary, optional hint, and optional body.
	lines := strings.SplitN(rest, "\n", 2)
	summary = lines[0]

	if len(lines) > 1 {
		remainder := lines[1]
		if strings.HasPrefix(remainder, "hint: ") {
			// Hint line — take up to next newline.
			hintRest := strings.SplitN(remainder[len("hint: "):], "\n", 2)
			hint = hintRest[0]
			if len(hintRest) > 1 {
				// Body is whatever follows — trim leading blank line.
				body = strings.TrimPrefix(hintRest[1], "\n")
			}
		} else {
			// No hint line — remainder is body.
			body = remainder
		}
	}

	return &ParsedToolError{
		Code:    code,
		Summary: summary,
		Hint:    hint,
		Body:    body,
	}
}
