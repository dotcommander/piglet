package memory

import (
	"encoding/json"
	"strings"
)

// truncateToolResults truncates large tool-result text content in messages
// to maxChars. This prevents bash/read output from dominating the LLM
// summarizer's token budget during compaction.
func truncateToolResults(msgs []wireMsg, maxChars int) {
	for i := range msgs {
		if msgs[i].Type != "tool_result" {
			continue
		}

		var tr wireToolResult
		if json.Unmarshal(msgs[i].Data, &tr) != nil {
			continue
		}

		modified := false
		for j := range tr.Content {
			if tr.Content[j].Type != "text" {
				continue
			}
			runes := []rune(tr.Content[j].Text)
			if len(runes) > maxChars {
				tr.Content[j].Text = string(runes[:maxChars]) + "\n[...truncated for compaction]"
				modified = true
			}
		}

		if modified {
			if data, err := json.Marshal(tr); err == nil {
				msgs[i].Data = data
			}
		}
	}
}

// extractPriorFileLists scans messages for prior compaction summaries
// containing <read-files> and <modified-files> XML tags and returns
// the accumulated file paths. This enables cumulative file tracking
// across compaction boundaries — files from earlier compactions are
// preserved when the next compaction runs.
func extractPriorFileLists(msgs []wireMsg) (readFiles, modifiedFiles []string) {
	readSet := make(map[string]struct{})
	modSet := make(map[string]struct{})

	for _, msg := range msgs {
		var m struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(msg.Data, &m) != nil || m.Content == "" {
			continue
		}

		for _, path := range parseXMLTag(m.Content, "read-files") {
			readSet[path] = struct{}{}
		}
		for _, path := range parseXMLTag(m.Content, "modified-files") {
			modSet[path] = struct{}{}
		}
	}

	for path := range readSet {
		readFiles = append(readFiles, path)
	}
	for path := range modSet {
		modifiedFiles = append(modifiedFiles, path)
	}
	return readFiles, modifiedFiles
}

// mergeFileLists appends prior compaction file paths into the current
// summary's <read-files> and <modified-files> tags, deduplicating entries.
func mergeFileLists(summary string, priorRead, priorModified []string) string {
	if len(priorRead) == 0 && len(priorModified) == 0 {
		return summary
	}

	// Parse existing tags from summary
	existingRead := parseXMLTag(summary, "read-files")
	existingMod := parseXMLTag(summary, "modified-files")

	// Merge with dedup
	mergedRead := dedup(append(existingRead, priorRead...))
	mergedMod := dedup(append(existingMod, priorModified...))

	// Strip existing tags from summary
	summary = stripXMLTag(summary, "read-files")
	summary = stripXMLTag(summary, "modified-files")
	summary = strings.TrimRight(summary, "\n")

	// Re-append merged tags
	var b strings.Builder
	b.WriteString(summary)

	if len(mergedRead) > 0 {
		b.WriteString("\n\n<read-files>\n")
		for _, f := range mergedRead {
			b.WriteString(f)
			b.WriteByte('\n')
		}
		b.WriteString("</read-files>")
	}
	if len(mergedMod) > 0 {
		b.WriteString("\n\n<modified-files>\n")
		for _, f := range mergedMod {
			b.WriteString(f)
			b.WriteByte('\n')
		}
		b.WriteString("</modified-files>")
	}

	return b.String()
}

// parseXMLTag extracts lines between <tag> and </tag> from text.
func parseXMLTag(text, tag string) []string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"

	start := strings.Index(text, open)
	if start < 0 {
		return nil
	}
	end := strings.Index(text[start:], close)
	if end < 0 {
		return nil
	}

	content := text[start+len(open) : start+end]
	var paths []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// stripXMLTag removes a <tag>...</tag> block from text.
func stripXMLTag(text, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"

	start := strings.Index(text, open)
	if start < 0 {
		return text
	}
	end := strings.Index(text[start:], close)
	if end < 0 {
		return text
	}

	return text[:start] + text[start+end+len(close):]
}

// dedup returns unique strings preserving first-seen order.
func dedup(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}
