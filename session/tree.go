package session

import (
	"fmt"
	"slices"

	"github.com/dotcommander/piglet/core"
)

// Label returns the label for an entry, or empty string.
func (s *Session) Label(entryID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.labels[entryID]
}

// Branch moves the leaf to an earlier entry, creating an in-place branch point.
// A branch_summary entry is written to persist the new leaf position across reloads.
func (s *Session) Branch(entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[entryID]; !ok {
		return fmt.Errorf("entry %s not found", entryID)
	}
	return s.writeBranchEntry(entryID, "")
}

// BranchWithSummary moves the leaf to an earlier entry and writes a
// branch_summary entry capturing context about the abandoned branch.
// The summary entry becomes the new leaf.
func (s *Session) BranchWithSummary(entryID, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[entryID]; !ok {
		return fmt.Errorf("entry %s not found", entryID)
	}
	return s.writeBranchEntry(entryID, summary)
}

// writeBranchEntry writes a branch_summary entry and moves the leaf.
// Must be called with s.mu held.
func (s *Session) writeBranchEntry(parentID, summary string) error {
	bs := BranchSummaryData{Summary: summary, FromID: s.leafID}
	data, err := marshalJSON(bs)
	if err != nil {
		return err
	}

	// Branch target is parentID, not the current leaf. Temporarily set leafID
	// so that commitNode attaches the entry to the correct parent.
	prevLeaf := s.leafID
	s.leafID = parentID
	_, err = s.commitNode(entryTypeBranchSummary, data, &node{typ: entryTypeBranchSummary})
	if err != nil {
		s.leafID = prevLeaf // restore on failure
	}
	return err
}

// EntryInfos returns info about all entries on the current branch (root to leaf).
func (s *Session) EntryInfos() []EntryInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.branchPath()

	// Compute children counts on the fly
	childCount := make(map[string]int, len(s.nodes))
	for _, n := range s.nodes {
		if n.parentID != "" {
			childCount[n.parentID]++
		}
	}

	infos := make([]EntryInfo, 0, len(path))
	for _, id := range path {
		n := s.nodes[id]
		if n == nil {
			continue
		}
		infos = append(infos, EntryInfo{
			ID:        id,
			ParentID:  n.parentID,
			Type:      n.typ,
			Timestamp: n.ts,
			Children:  childCount[id],
		})
	}
	return infos
}

// FullTree returns every entry in the session as a flat list ordered by DFS traversal.
// Active path entries are marked. Children are sorted with the active subtree first,
// then by timestamp (oldest first).
func (s *Session) FullTree() []TreeNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.nodes) == 0 {
		return nil
	}

	// Build parent → children map
	children := make(map[string][]string, len(s.nodes))
	var roots []string
	for id, n := range s.nodes {
		if n.parentID == "" {
			roots = append(roots, id)
		} else {
			children[n.parentID] = append(children[n.parentID], id)
		}
	}

	// Active path set
	activePath := make(map[string]bool, len(s.nodes))
	for _, id := range s.branchPath() {
		activePath[id] = true
	}

	// Sort children: active-subtree first, then oldest-first
	for parentID, kids := range children {
		slices.SortFunc(kids, func(a, b string) int {
			aActive := s.isInActiveSubtree(a, activePath, children)
			bActive := s.isInActiveSubtree(b, activePath, children)
			if aActive != bActive {
				if aActive {
					return -1
				}
				return 1
			}
			return s.nodes[a].ts.Compare(s.nodes[b].ts)
		})
		children[parentID] = kids
	}

	// Sort roots the same way
	slices.SortFunc(roots, func(a, b string) int {
		return s.nodes[a].ts.Compare(s.nodes[b].ts)
	})

	// DFS
	var result []TreeNode
	var dfs func(id string, depth int)
	dfs = func(id string, depth int) {
		n := s.nodes[id]
		if n == nil {
			return
		}
		result = append(result, TreeNode{
			ID:           id,
			ParentID:     n.parentID,
			Type:         n.typ,
			Timestamp:    n.ts,
			Children:     len(children[id]),
			OnActivePath: activePath[id],
			Depth:        depth,
			Preview:      s.nodePreview(n),
			Label:        s.labels[id],
			TokensBefore: n.tokensBefore,
		})
		for _, kid := range children[id] {
			dfs(kid, depth+1)
		}
	}
	for _, root := range roots {
		dfs(root, 0)
	}

	return result
}

// isInActiveSubtree returns true if id or any descendant is on the active path.
func (s *Session) isInActiveSubtree(id string, activePath map[string]bool, children map[string][]string) bool {
	if activePath[id] {
		return true
	}
	for _, kid := range children[id] {
		if s.isInActiveSubtree(kid, activePath, children) {
			return true
		}
	}
	return false
}

// nodePreview returns a short text preview for a node.
func (s *Session) nodePreview(n *node) string {
	if n.message == nil {
		return ""
	}
	switch m := n.message.(type) {
	case *core.UserMessage:
		return truncatePreview(m.Content, 60)
	case *core.AssistantMessage:
		for _, c := range m.Content {
			if tc, ok := c.(core.TextContent); ok {
				return truncatePreview(tc.Text, 60)
			}
		}
	}
	return ""
}

// truncatePreview truncates a string to n runes, appending "..." if truncated.
func truncatePreview(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// buildBranch walks from leaf to root and builds the message list for the
// current branch. Compaction entries reset the message list.
// Must be called with s.mu held.
func (s *Session) buildBranch() []core.Message {
	path := s.branchPath()

	var msgs []core.Message
	for _, id := range path {
		n := s.nodes[id]
		if n == nil {
			continue
		}
		switch {
		case n.compact != nil:
			msgs = msgs[:0]
			msgs = append(msgs, n.compact...)
		case n.message != nil:
			msgs = append(msgs, n.message)
		}
	}
	return msgs
}

// branchPath returns entry IDs from root to leaf for the current branch.
// Must be called with s.mu held.
func (s *Session) branchPath() []string {
	if s.leafID == "" {
		return nil
	}
	var path []string
	current := s.leafID
	for current != "" {
		path = append(path, current)
		n := s.nodes[current]
		if n == nil {
			break
		}
		current = n.parentID
	}
	slices.Reverse(path)
	return path
}
