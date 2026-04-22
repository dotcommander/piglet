package subagent

import (
	"strings"
	"sync"
	"time"
)

const (
	recentTaskTTL = 30 * time.Minute
	prunePeriod   = 5 * time.Minute
)

// recentTask is a completed subagent dispatch cached for dedup.
type recentTask struct {
	result      string
	completedAt time.Time
}

// dedupCache stores completed subagent results keyed on normalized prompt.
// Lookups are constant-time; pruning runs on a lazily-started ticker.
type dedupCache struct {
	mu      sync.Mutex
	entries map[string]recentTask
	ticker  *time.Ticker
	stop    chan struct{}
}

// normalizePrompt produces the dedup key — lowercased, whitespace-collapsed.
// Prompts that differ only in casing or spacing collide; intentionally narrow
// (won't catch paraphrases — just exact and near-exact repeats).
func normalizePrompt(prompt string) string {
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

// lookup returns the cached result and true if a non-expired entry exists.
func (c *dedupCache) lookup(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Since(e.completedAt) > recentTaskTTL {
		delete(c.entries, key)
		return "", false
	}
	return e.result, true
}

// store records a completed result and lazily starts the prune ticker.
func (c *dedupCache) store(key, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]recentTask)
	}
	c.entries[key] = recentTask{result: result, completedAt: time.Now()}
	if c.ticker == nil {
		c.ticker = time.NewTicker(prunePeriod)
		c.stop = make(chan struct{})
		go c.pruneLoop()
	}
}

// pruneLoop removes expired entries until stop is signalled.
func (c *dedupCache) pruneLoop() {
	for {
		select {
		case <-c.ticker.C:
			c.prune()
		case <-c.stop:
			return
		}
	}
}

// prune removes entries older than recentTaskTTL.
func (c *dedupCache) prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-recentTaskTTL)
	for k, e := range c.entries {
		if e.completedAt.Before(cutoff) {
			delete(c.entries, k)
		}
	}
}
