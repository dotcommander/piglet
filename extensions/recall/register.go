package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

const (
	maxExtractBytes    = 32 * 1024 // 32 KB per session for indexing
	hookScoreThreshold = 0.3
	hookMinWords       = 20
	searchExcerptLen   = 200
	defaultSearchLimit = 3
)

// recallState holds mutable state shared across tool and command handlers.
type recallState struct {
	idx       *Index
	indexPath string
}

// Register wires the recall extension into the pack.
func Register(e *sdk.Extension) {
	st := &recallState{}

	e.OnInitAppend(func(x *sdk.Extension) {
		dir, err := xdg.ExtensionDir("recall")
		if err != nil {
			x.Log("error", fmt.Sprintf("[recall] ExtensionDir: %v", err))
			st.idx = NewIndex(500)
			return
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			x.Log("error", fmt.Sprintf("[recall] MkdirAll: %v", err))
		}
		st.indexPath = filepath.Join(dir, "index.gob")
		loaded, err := LoadIndex(st.indexPath)
		if err != nil {
			st.idx = NewIndex(500)
		} else {
			st.idx = loaded
		}
		docCount, termCount := st.idx.Stats()
		x.Log("debug", fmt.Sprintf("[recall] index loaded: %d docs, %d terms", docCount, termCount))
	})

	registerEventHandler(e, st)
	registerCommand(e, st)
	registerTool(e, st)
	registerHook(e, st)
}

// registerEventHandler indexes the session at EventAgentEnd.
func registerEventHandler(e *sdk.Extension, st *recallState) {
	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "recall-index",
		Priority: 300,
		Events:   []string{"EventAgentEnd"},
		Handle: func(ctx context.Context, _ string, data json.RawMessage) *sdk.Action {
			if st.idx == nil {
				return nil
			}

			sessionID := os.Getenv("PIGLET_SESSION_ID")
			if sessionID == "" {
				return nil
			}

			var evt struct {
				Messages []json.RawMessage `json:"Messages"`
			}
			if err := json.Unmarshal(data, &evt); err != nil || len(evt.Messages) == 0 {
				return nil
			}

			text := formatMessagesText(evt.Messages)
			if text == "" {
				return nil
			}

			path, title := resolveSessionMeta(ctx, e, sessionID)
			st.idx.AddDocument(sessionID, path, title, text)

			if st.indexPath != "" {
				if err := st.idx.Save(st.indexPath); err != nil {
					e.Log("error", fmt.Sprintf("[recall] save index: %v", err))
				}
			}
			return nil
		},
	})
}

// registerCommand wires /recall with subcommand dispatch.
func registerCommand(e *sdk.Extension, st *recallState) {
	e.RegisterCommand(sdk.CommandDef{
		Name:        "recall",
		Description: "Search session history by content (recall <query>, rebuild, stats)",
		Handler: func(ctx context.Context, args string) error {
			sub := strings.TrimSpace(args)
			switch {
			case sub == "stats":
				return handleStats(e, st)
			case sub == "rebuild":
				return handleRebuild(ctx, e, st)
			case sub != "":
				return handleSearch(ctx, e, st, sub)
			default:
				e.ShowMessage("Usage: /recall <query> | /recall rebuild | /recall stats")
			}
			return nil
		},
	})
}

// registerTool wires the recall_search tool.
func registerTool(e *sdk.Extension, st *recallState) {
	e.RegisterTool(sdk.ToolDef{
		Name:        "recall_search",
		Description: "Search past sessions by content using TF-IDF. Returns matching session excerpts useful for recovering prior context.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query — keywords or phrases to find in past sessions",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum results to return (default 3)",
				},
			},
			"required": []any{"query"},
		},
		PromptHint: "Search past sessions for relevant context",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if st.idx == nil {
				return sdk.ErrorResult("recall index not available"), nil
			}
			query, _ := args["query"].(string)
			if query == "" {
				return sdk.ErrorResult("query is required"), nil
			}
			limit := defaultSearchLimit
			if l, ok := args["limit"].(float64); ok && int(l) > 0 {
				limit = int(l)
			}

			results := st.idx.Search(query, limit)
			if len(results) == 0 {
				return sdk.TextResult("No matching sessions found for: " + query), nil
			}

			text := buildResultsText(results)
			return sdk.TextResult(text), nil
		},
	})
}

// registerHook wires the auto-recall message hook.
func registerHook(e *sdk.Extension, st *recallState) {
	e.RegisterMessageHook(sdk.MessageHookDef{
		Name:     "recall-auto",
		Priority: 800,
		OnMessage: func(_ context.Context, msg string) (string, error) {
			if st.idx == nil {
				return "", nil
			}
			if wordCount(msg) < hookMinWords {
				return "", nil
			}

			results := st.idx.Search(msg, 1)
			if len(results) == 0 || results[0].Score < hookScoreThreshold {
				return "", nil
			}

			top := results[0]
			excerpt := readExcerpt(top.Path, searchExcerptLen)
			if excerpt == "" {
				return "", nil
			}

			label := top.Title
			if label == "" {
				label = top.SessionID
				if len(label) > 8 {
					label = label[:8]
				}
			}
			return fmt.Sprintf("# Prior Context (session: %s)\n\n%s", label, excerpt), nil
		},
	})
}

// handleSearch executes a /recall <query> command.
func handleSearch(_ context.Context, e *sdk.Extension, st *recallState, query string) error {
	if st.idx == nil {
		e.ShowMessage("recall index not available")
		return nil
	}

	results := st.idx.Search(query, defaultSearchLimit)
	if len(results) == 0 {
		e.ShowMessage("No sessions found matching: " + query)
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Recall: %q (%d results)\n\n", query, len(results))
	for i, r := range results {
		label := r.Title
		if label == "" {
			label = r.SessionID
			if len(label) > 8 {
				label = label[:8]
			}
		}
		fmt.Fprintf(&b, "%d. %s (score: %.4f)\n", i+1, label, r.Score)
		excerpt := readExcerpt(r.Path, searchExcerptLen)
		if excerpt != "" {
			fmt.Fprintf(&b, "   %s\n", strings.ReplaceAll(strings.TrimSpace(excerpt), "\n", " "))
		}
	}
	e.ShowMessage(b.String())
	return nil
}

// handleRebuild re-indexes all known sessions.
func handleRebuild(ctx context.Context, e *sdk.Extension, st *recallState) error {
	sessions, err := e.Sessions(ctx)
	if err != nil {
		e.ShowMessage("rebuild failed: " + err.Error())
		return nil
	}

	fresh := NewIndex(500)
	count := 0
	for _, s := range sessions {
		if s.Path == "" {
			continue
		}
		text, err := ExtractSessionText(s.Path, maxExtractBytes)
		if err != nil || text == "" {
			continue
		}
		fresh.AddDocument(s.ID, s.Path, s.Title, text)
		count++
	}

	st.idx = fresh
	if st.indexPath != "" {
		if err := st.idx.Save(st.indexPath); err != nil {
			e.ShowMessage(fmt.Sprintf("rebuild indexed %d sessions but save failed: %v", count, err))
			return nil
		}
	}

	docs, terms := st.idx.Stats()
	e.ShowMessage(fmt.Sprintf("Rebuild complete: %d sessions indexed, %d unique terms", docs, terms))
	return nil
}

// handleStats shows index statistics.
func handleStats(e *sdk.Extension, st *recallState) error {
	if st.idx == nil {
		e.ShowMessage("recall index not available")
		return nil
	}
	docs, terms := st.idx.Stats()
	e.ShowMessage(fmt.Sprintf("Recall index: %d sessions, %d unique terms", docs, terms))
	return nil
}

// resolveSessionMeta returns the path and title for sessionID by looking it up
// in e.Sessions. Falls back to empty strings if sessions cannot be fetched.
func resolveSessionMeta(ctx context.Context, e *sdk.Extension, sessionID string) (path, title string) {
	sessions, err := e.Sessions(ctx)
	if err != nil {
		return "", ""
	}
	for _, s := range sessions {
		if s.ID == sessionID {
			return s.Path, s.Title
		}
	}
	return "", ""
}

// formatMessagesText converts EventAgentEnd messages to a plain text string.
func formatMessagesText(messages []json.RawMessage) string {
	var b strings.Builder
	for _, raw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &msg) != nil || msg.Role == "" {
			continue
		}
		text := extractTextContent(msg.Content)
		if text == "" {
			continue
		}
		role := strings.Title(msg.Role) //nolint:staticcheck // acceptable for display
		fmt.Fprintf(&b, "%s: %s\n", role, text)
	}
	return b.String()
}

// buildResultsText formats search results as a readable string.
func buildResultsText(results []SearchResult) string {
	var b strings.Builder
	for i, r := range results {
		label := r.Title
		if label == "" {
			label = r.SessionID
			if len(label) > 8 {
				label = label[:8]
			}
		}
		fmt.Fprintf(&b, "%d. %s (score: %.4f)\n", i+1, label, r.Score)
		excerpt := readExcerpt(r.Path, searchExcerptLen)
		if excerpt != "" {
			fmt.Fprintf(&b, "   %s\n", strings.ReplaceAll(strings.TrimSpace(excerpt), "\n", " "))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// readExcerpt reads the first maxChars characters of text from the session file.
func readExcerpt(path string, maxChars int) string {
	if path == "" {
		return ""
	}
	text, err := ExtractSessionText(path, maxChars*4) // bytes approx
	if err != nil {
		return ""
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		return string(runes[:maxChars])
	}
	return text
}

// wordCount returns the approximate number of words in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}
