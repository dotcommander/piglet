package bulk

import "time"

// Source type constants for tool schema.
const (
	SourceGitRepos = "git_repos"
	SourceDirs     = "dirs"
	SourceFiles    = "files"
	SourceList     = "list"
)

// Item is a single unit of work.
type Item struct {
	Name string            `json:"name"`
	Path string            `json:"path"`
	Meta map[string]string `json:"meta,omitzero"`
}

// Result is the outcome of running one command on one item.
type Result struct {
	Item   string `json:"item"`
	Path   string `json:"path"`
	Status string `json:"status"` // "ok", "error", "skipped"
	Output string `json:"output"`
}

// Summary is the aggregate output of a bulk operation.
type Summary struct {
	TotalCollected int      `json:"total_collected"`
	MatchedFilter  int      `json:"matched_filter"`
	Results        []Result `json:"results"`
	Message        string   `json:"message"`
}

// Config controls execution behavior.
type Config struct {
	Concurrency int           // max parallel executions, default 8
	Timeout     time.Duration // per-item timeout, default 30s
	DryRun      bool          // collect and filter but don't execute
}

// Command defines what to run on each item.
// Template vars: {path}, {name}, {dir}, {basename}.
type Command struct {
	Template string // shell command, e.g. "git push" or "make -C {path} clean"
	Shell    string // default "sh"
}

// defaults fills zero-value fields with sensible defaults.
func (c *Config) defaults() {
	if c.Concurrency <= 0 {
		c.Concurrency = 8
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
}
