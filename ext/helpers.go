package ext

import "github.com/dotcommander/piglet/core"

// TextResult builds a ToolResult containing a single text block.
func TextResult(text string) *core.ToolResult {
	return &core.ToolResult{
		Content: []core.ContentBlock{core.TextContent{Text: text}},
	}
}

// StringArg extracts a string argument from a tool call's args map.
func StringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// IntArg extracts an integer argument from a tool call's args map.
// JSON numbers decode as float64, so both float64 and int are handled.
func IntArg(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return fallback
	}
}

// BoolArg extracts a boolean argument from a tool call's args map.
func BoolArg(args map[string]any, key string, fallback bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return fallback
}
