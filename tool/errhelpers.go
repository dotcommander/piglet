package tool

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/errfmt"
)

// toolStatErr classifies errors from os.Stat — same mapping as read errors.
func toolStatErr(path string, err error) *core.ToolResult {
	return toolReadErr(path, err)
}

// toolReadErr classifies errors from reading a file.
func toolReadErr(path string, err error) *core.ToolResult {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errfmt.ToolErr(errfmt.ToolErrFileNotFound,
			fmt.Sprintf("file not found: %s", path),
			"verify the path; use `find` or repomap to locate files")
	case errors.Is(err, fs.ErrPermission):
		return errfmt.ToolErr(errfmt.ToolErrPermissionDenied,
			fmt.Sprintf("permission denied: %s", path),
			"file is not readable by piglet")
	default:
		return errfmt.ToolErr(errfmt.ToolErrIO,
			fmt.Sprintf("read failed: %s: %v", path, err),
			"")
	}
}

// toolWriteErr classifies errors from writing (atomicWrite, MkdirAll).
// verb is the operation name ("create directory", "write file") for the summary.
func toolWriteErr(path string, err error, verb string) *core.ToolResult {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return errfmt.ToolErr(errfmt.ToolErrPermissionDenied,
			fmt.Sprintf("permission denied: %s", path),
			fmt.Sprintf("cannot %s — check filesystem permissions", verb))
	default:
		return errfmt.ToolErr(errfmt.ToolErrIO,
			fmt.Sprintf("%s failed: %s: %v", verb, path, err),
			"")
	}
}

// isExitError reports whether err (or an error it wraps) is *exec.ExitError.
func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

// toolBashErr wraps a coded error with an appended output body.
// outputBody is appended after a blank line so the header remains scannable.
func toolBashErr(code errfmt.ToolErrorCode, summary, hint, outputBody string) *core.ToolResult {
	res := errfmt.ToolErr(code, summary, hint)
	if outputBody == "" {
		return res
	}
	// Append output after a blank line so the header remains scannable.
	tc := res.Content[0].(core.TextContent)
	tc.Text += "\n\n" + outputBody
	res.Content[0] = tc
	return res
}
