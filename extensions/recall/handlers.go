package recall

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/dotcommander/piglet/sdk"
)

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
