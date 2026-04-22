// Package sdk — coded tool errors.
package sdk

import (
	"fmt"
	"strings"
)

// ToolErrorCode is a stable machine-readable identifier for tool failures.
// Extensions should prefer the standard codes below, but any uppercase
// snake_case string is valid. The host's classifier reads both the embedded
// text prefix and the wire ErrorCode field.
type ToolErrorCode = string

// Standard codes — keep in sync with errfmt.ToolErrorCode.
const (
	ToolErrInvalidArgs      ToolErrorCode = "INVALID_ARGS"
	ToolErrFileNotFound     ToolErrorCode = "FILE_NOT_FOUND"
	ToolErrFileStale        ToolErrorCode = "FILE_STALE"
	ToolErrFileTooLarge     ToolErrorCode = "FILE_TOO_LARGE"
	ToolErrNotRegularFile   ToolErrorCode = "NOT_REGULAR_FILE"
	ToolErrPermissionDenied ToolErrorCode = "PERMISSION_DENIED"
	ToolErrNotUnique        ToolErrorCode = "NOT_UNIQUE"
	ToolErrTimeout          ToolErrorCode = "TIMEOUT"
	ToolErrExitNonzero      ToolErrorCode = "EXIT_NONZERO"
	ToolErrIO               ToolErrorCode = "IO_ERROR"
	ToolErrInternal         ToolErrorCode = "INTERNAL"
)

// ToolErr constructs an errored ToolResult with a canonical
// "[error:CODE] summary\nhint: hint" text body, IsError=true, and the
// structured ErrorCode/ErrorHint fields set. Use this instead of
// ErrorResult when you want the LLM to see a machine-readable classifier.
func ToolErr(code ToolErrorCode, summary, hint string) *ToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "[error:%s] %s", code, summary)
	if hint != "" {
		b.WriteString("\nhint: ")
		b.WriteString(hint)
	}
	return &ToolResult{
		Content:   []ContentBlock{{Type: "text", Text: b.String()}},
		IsError:   true,
		ErrorCode: code,
		ErrorHint: hint,
	}
}
