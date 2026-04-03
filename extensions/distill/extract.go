package distill

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const maxFormatChars = 6000

// scoreComplexity analyzes messages for session complexity.
// Returns a score based on message count, tool usage, and error recovery patterns.
// Messages come from EventAgentEnd payload as []json.RawMessage.
func scoreComplexity(messages []json.RawMessage) int {
	score := len(messages)

	var prevHadError bool
	for _, raw := range messages {
		score += countToolUseBlocks(raw) * 2
		if prevHadError && hasSuccessfulToolCall(raw) {
			score += 3
		}
		prevHadError = isAssistantWithError(raw)
	}

	return score
}

// countToolUseBlocks counts tool_use content blocks in a message.
func countToolUseBlocks(raw json.RawMessage) int {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return 0
	}

	// Content may be a string or []block
	if len(msg.Content) > 0 && msg.Content[0] == '"' {
		return 0
	}

	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return 0
	}

	count := 0
	for _, b := range blocks {
		if b.Type == "tool_use" {
			count++
		}
	}
	return count
}

// isAssistantWithError returns true if msg is an assistant message containing "error".
// Checks only text blocks — tool names and tool results are ignored.
func isAssistantWithError(raw json.RawMessage) bool {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || msg.Role != "assistant" {
		return false
	}
	return textContainsError(msg.Content)
}

// textContainsError checks only text blocks (not tool names/results) for "error".
func textContainsError(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return strings.Contains(strings.ToLower(s), "error")
		}
		return false
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.Contains(strings.ToLower(b.Text), "error") {
			return true
		}
	}
	return false
}

// hasSuccessfulToolCall returns true if msg contains a tool_use block (successful follow-up).
func hasSuccessfulToolCall(raw json.RawMessage) bool {
	return countToolUseBlocks(raw) > 0
}

// formatMessages converts raw JSON messages into a readable transcript for the LLM.
// Truncates to maxBytes.
func formatMessages(messages []json.RawMessage, maxBytes int) string {
	var b strings.Builder
	for _, raw := range messages {
		line := formatMessage(raw)
		if line == "" {
			continue
		}
		if b.Len()+len(line) > maxBytes {
			remaining := maxBytes - b.Len()
			if remaining > 0 && remaining < len(line) {
				b.WriteString(line[:remaining])
			}
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

// formatMessage renders a single message as a readable line.
func formatMessage(raw json.RawMessage) string {
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || msg.Role == "" {
		return ""
	}

	role := msg.Role
	if len(role) > 0 && role[0] >= 'a' && role[0] <= 'z' {
		role = string(role[0]-32) + role[1:]
	}
	content := extractTextContent(msg.Content)
	if content == "" {
		return ""
	}
	return role + ": " + content + "\n"
}

// extractTextContent pulls readable text out of a content field (string or []block).
func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// String content
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}

	// Block array
	var blocks []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Name  string `json:"name"`
		Input any    `json:"input"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		case "tool_use":
			parts = append(parts, fmt.Sprintf("[tool: %s]", b.Name))
		case "tool_result":
			parts = append(parts, "[tool result]")
		}
	}
	return strings.Join(parts, " ")
}

// writeSkill writes a skill file to ~/.config/piglet/skills/.
// Content should be a complete markdown file with YAML frontmatter.
// Returns the path written.
func writeSkill(content string) (string, error) {
	base, err := xdg.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return writeSkillTo(filepath.Join(base, "skills"), content)
}

// writeSkillTo writes a skill file to an explicit directory.
// Extracted for testability — tests pass t.TempDir() instead of the real skills dir.
func writeSkillTo(dir, content string) (string, error) {
	name := extractFrontmatterName(content)
	if name == "" {
		name = "distilled-skill"
		content = wrapWithFrontmatter(content, name)
	}

	name = sanitizeName(name)
	path := filepath.Join(dir, name+".md")
	if err := xdg.WriteFileAtomic(path, []byte(content)); err != nil {
		return "", fmt.Errorf("write skill %s: %w", path, err)
	}
	return path, nil
}

var frontmatterNameRe = regexp.MustCompile(`(?m)^name:\s*(.+)$`)

// extractFrontmatterName parses the name field from YAML frontmatter.
func extractFrontmatterName(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return ""
	}
	// Find closing --- to limit search to frontmatter section only.
	rest := trimmed[3:]
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx < 0 {
		closeIdx = len(rest)
	}
	frontmatter := rest[:closeIdx]
	m := frontmatterNameRe.FindStringSubmatch(frontmatter)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]`)
var multiHyphenRe = regexp.MustCompile(`-{2,}`)

// sanitizeName produces a safe filename: lowercase, hyphens, no special chars.
func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumRe.ReplaceAllString(s, "")
	s = multiHyphenRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "distilled-skill"
	}
	return s
}

// wrapWithFrontmatter wraps bare content with minimal frontmatter.
func wrapWithFrontmatter(content, name string) string {
	fm := fmt.Sprintf("---\nname: %s\ndescription: Auto-extracted skill\ntriggers:\n  - skill\nsource: distill\n---\n\n", name)
	return fm + content
}

// distillSession runs the full extraction pipeline on messages.
func distillSession(ctx context.Context, e *sdk.Extension, messages []json.RawMessage) (string, error) {
	transcript := formatMessages(messages, maxFormatChars)
	if transcript == "" {
		return "", fmt.Errorf("no transcript content to distill")
	}

	prompt := xdg.LoadOrCreateExt("distill", "extract-prompt.md", strings.TrimSpace(defaultPrompt))

	resp, err := e.Chat(ctx, sdk.ChatRequest{
		System:    prompt,
		Messages:  []sdk.ChatMessage{{Role: "user", Content: transcript}},
		Model:     "small",
		MaxTokens: 2048,
	})
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	text := strings.TrimSpace(resp.Text)
	if text == "" || strings.EqualFold(text, "SKIP") {
		return "", fmt.Errorf("model skipped: no reusable skill found")
	}

	path, err := writeSkill(text)
	if err != nil {
		return "", fmt.Errorf("write skill: %w", err)
	}
	return path, nil
}

// readSessionMessages reads a session JSONL file and returns the message entries.
// Meta-type entries are skipped. Returns the data field of each non-meta entry.
func readSessionMessages(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session %s: %w", path, err)
	}
	defer f.Close()

	var messages []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var entry struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.Type == "meta" || len(entry.Data) == 0 {
			continue
		}
		messages = append(messages, entry.Data)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session %s: %w", path, err)
	}
	return messages, nil
}
