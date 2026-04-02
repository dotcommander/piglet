package shell

import (
	"path/filepath"
	"strings"
)

// ToolSummary returns a concise display string for a tool call.
// Format: "toolname: detail" or just "toolname" if no meaningful detail.
func ToolSummary(toolName string, args map[string]any) string {
	detail := toolDetail(toolName, args)
	if detail == "" {
		return toolName
	}
	return toolName + ": " + detail
}

func toolDetail(toolName string, args map[string]any) string {
	switch toolName {
	case "bash":
		return bashDetail(args)
	case "read", "write", "edit":
		return fileDetail(args)
	case "grep":
		return grepDetail(args)
	case "find":
		return findDetail(args)
	case "ls":
		return strArg(args, "path")
	default:
		return ""
	}
}

func bashDetail(args map[string]any) string {
	cmd := strArg(args, "command")
	if cmd == "" {
		return ""
	}
	// Take first line only for multi-line commands.
	if i := strings.IndexByte(cmd, '\n'); i >= 0 {
		cmd = cmd[:i]
	}
	return TruncateRunes(strings.TrimSpace(cmd), 80)
}

func fileDetail(args map[string]any) string {
	p := strArg(args, "path")
	if p == "" {
		p = strArg(args, "file_path")
	}
	if p == "" {
		return ""
	}
	// Show last two path components for context without excessive length.
	dir := filepath.Base(filepath.Dir(p))
	base := filepath.Base(p)
	if dir == "." || dir == "/" {
		return base
	}
	return dir + "/" + base
}

func grepDetail(args map[string]any) string {
	pattern := strArg(args, "pattern")
	if pattern == "" {
		return ""
	}
	detail := TruncateRunes(pattern, 40)
	if p := strArg(args, "path"); p != "" {
		detail += " in " + filepath.Base(p)
	}
	return detail
}

func findDetail(args map[string]any) string {
	pattern := strArg(args, "pattern")
	if pattern != "" {
		return pattern
	}
	return strArg(args, "path")
}

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
