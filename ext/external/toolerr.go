package external

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/core"
)

// ensureCodedErrorPrefix ensures the first text block of content carries an
// [error:CODE] prefix when errorCode is set. No-op if the prefix is already
// present. Lets external extensions set ErrorCode without manually formatting
// the header.
func ensureCodedErrorPrefix(blocks []core.ContentBlock, code, hint string) []core.ContentBlock {
	if code == "" || len(blocks) == 0 {
		return blocks
	}
	tc, ok := blocks[0].(core.TextContent)
	if !ok {
		return blocks
	}
	if strings.HasPrefix(tc.Text, "[error:") {
		return blocks // already formatted
	}
	var prefix string
	if hint != "" {
		prefix = fmt.Sprintf("[error:%s] %s\nhint: %s\n", code, tc.Text, hint)
	} else {
		prefix = fmt.Sprintf("[error:%s] %s\n", code, tc.Text)
	}
	// NOTE: this rewrites the first text block's full contents — the summary
	// becomes the previous text (which may be a whole stderr dump). This is
	// acceptable because external extensions that skip the prefix are expected
	// to have a short text block; the ergonomic path is sdk.ToolErr.
	blocks[0] = core.TextContent{Text: strings.TrimSuffix(prefix, "\n")}
	return blocks
}
