// Package toolresult provides helpers for reading and modifying the "details"
// payload that the piglet SDK passes through interceptor After callbacks.
// The payload may be a plain string, a map with a "content" key (string or
// block array), or a map with an "output" key.
package toolresult

// ExtractText returns the first text value from an interceptor details payload.
// Returns ("", false) when no text is found.
func ExtractText(details any) (string, bool) {
	m, ok := details.(map[string]any)
	if !ok {
		s, ok := details.(string)
		return s, ok
	}

	content, ok := m["content"]
	if !ok {
		output, ok := m["output"].(string)
		return output, ok
	}

	if s, ok := content.(string); ok {
		return s, true
	}

	blocks, ok := content.([]any)
	if !ok {
		return "", false
	}

	for _, block := range blocks {
		bm, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := bm["text"].(string); ok {
			return text, true
		}
	}

	return "", false
}

// ReplaceText returns a copy of details with the first text value replaced by
// replacement. The original details value is never mutated.
func ReplaceText(details any, replacement string) any {
	m, ok := details.(map[string]any)
	if !ok {
		return replacement
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}

	if content, ok := result["content"]; ok {
		if blocks, ok := content.([]any); ok && len(blocks) > 0 {
			newBlocks := make([]any, len(blocks))
			copy(newBlocks, blocks)
			if bm, ok := newBlocks[0].(map[string]any); ok {
				newBlock := make(map[string]any, len(bm))
				for k, v := range bm {
					newBlock[k] = v
				}
				newBlock["text"] = replacement
				newBlocks[0] = newBlock
			}
			result["content"] = newBlocks
			return result
		}
		if _, ok := content.(string); ok {
			result["content"] = replacement
			return result
		}
	}

	if _, ok := result["output"]; ok {
		result["output"] = replacement
		return result
	}

	return result
}
