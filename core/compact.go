package core

import (
	"context"
	"time"
)

// maybeCompact triggers background compaction if token usage exceeds the configured threshold.
func (a *Agent) maybeCompact() {
	a.mu.RLock()
	// Use the most recent assistant message's InputTokens — that IS the current
	// context window size (the API reports full context per turn, not incremental).
	var total int
	for i := len(a.messages) - 1; i >= 0; i-- {
		if am, ok := a.messages[i].(*AssistantMessage); ok {
			total = am.Usage.InputTokens
			break
		}
	}
	threshold := a.cfg.CompactAt
	msgCount := len(a.messages)
	var msgs []Message
	if total >= threshold && msgCount >= 8 {
		msgs = make([]Message, msgCount)
		copy(msgs, a.messages)
	}
	snapshotLen := msgCount
	a.mu.RUnlock()

	if msgs == nil {
		return
	}

	a.compactMu.Lock()
	if a.compacting {
		a.compactMu.Unlock()
		return
	}
	a.compacting = true
	a.compactMu.Unlock()

	a.compactWg.Add(1)
	go a.runCompaction(msgs, snapshotLen, total)
}

func (a *Agent) runCompaction(msgs []Message, snapshotLen, tokenCount int) {
	defer a.compactWg.Done()
	defer func() {
		a.compactMu.Lock()
		a.compacting = false
		a.compactMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	compacted, err := a.cfg.OnCompact(ctx, msgs)
	if err != nil || len(compacted) == 0 {
		return
	}

	// Preserve any messages appended while compaction was running.
	a.mu.Lock()
	if snapshotLen > len(a.messages) {
		snapshotLen = len(a.messages)
	}
	tail := a.messages[snapshotLen:]
	if len(tail) == 0 {
		a.messages = compacted
	} else {
		merged := make([]Message, len(compacted)+len(tail))
		copy(merged, compacted)
		copy(merged[len(compacted):], tail)
		a.messages = merged
	}
	a.mu.Unlock()

	a.emit(EventCompact{Before: len(msgs), After: len(compacted), TokensAtCompact: tokenCount})
}

// enforceMessageCap drops oldest messages (keeping the first) when over MaxMessages.
func (a *Agent) enforceMessageCap() {
	a.mu.Lock()
	defer a.mu.Unlock()

	maxMsg := a.cfg.MaxMessages
	if len(a.messages) <= maxMsg {
		return
	}

	// Keep first message + last (maxMsg-1) messages
	trimmed := make([]Message, 0, maxMsg)
	trimmed = append(trimmed, a.messages[0])
	trimmed = append(trimmed, a.messages[len(a.messages)-maxMsg+1:]...)
	a.messages = trimmed
}

