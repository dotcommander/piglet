package route

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

// FeedbackEntry records a routing accuracy observation.
type FeedbackEntry struct {
	Timestamp  string `json:"ts"`
	PromptHash string `json:"prompt_hash"`
	Prompt     string `json:"prompt"`
	Component  string `json:"component"`
	Correct    bool   `json:"correct"`
}

// LearnedTriggers holds triggers and anti-triggers learned from feedback.
type LearnedTriggers struct {
	Triggers     map[string][]string `json:"triggers"`      // component -> trigger tokens
	AntiTriggers map[string][]string `json:"anti_triggers"` // component -> anti-trigger tokens
}

// FeedbackStore manages feedback recording and learned trigger generation.
type FeedbackStore struct {
	dir string
}

// NewFeedbackStore creates a store at the given config directory.
func NewFeedbackStore(dir string) *FeedbackStore {
	return &FeedbackStore{dir: dir}
}

// feedbackDir returns the feedback store directory, creating if needed.
func feedbackDir() (string, error) {
	dir, err := xdg.ExtensionDir("route")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Record appends a feedback entry to the JSONL log.
func (fs *FeedbackStore) Record(prompt, component string, correct bool) error {
	entry := FeedbackEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		PromptHash: hashPrompt(prompt),
		Prompt:     prompt,
		Component:  component,
		Correct:    correct,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	path := filepath.Join(fs.dir, "feedback.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open feedback: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write feedback: %w", err)
	}
	return nil
}

// Learn processes all feedback entries and generates learned triggers.
// Correct feedback adds tokens as triggers; wrong feedback adds anti-triggers.
func (fs *FeedbackStore) Learn() (*LearnedTriggers, error) {
	path := filepath.Join(fs.dir, "feedback.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LearnedTriggers{
				Triggers:     make(map[string][]string),
				AntiTriggers: make(map[string][]string),
			}, nil
		}
		return nil, fmt.Errorf("open feedback: %w", err)
	}
	defer f.Close()

	lt := &LearnedTriggers{
		Triggers:     make(map[string][]string),
		AntiTriggers: make(map[string][]string),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry FeedbackEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		tokens := Tokenize(entry.Prompt)
		if entry.Correct {
			existing := lt.Triggers[entry.Component]
			merged := mergeTokens(existing, tokens, 20)
			lt.Triggers[entry.Component] = merged
		} else {
			existing := lt.AntiTriggers[entry.Component]
			merged := mergeTokens(existing, tokens, 10)
			lt.AntiTriggers[entry.Component] = merged
		}
	}

	// Save learned triggers
	outPath := filepath.Join(fs.dir, "learned-triggers.json")
	data, err := json.MarshalIndent(lt, "", "  ")
	if err != nil {
		return lt, fmt.Errorf("marshal learned triggers: %w", err)
	}
	if err := xdg.WriteFileAtomic(outPath, data); err != nil {
		return lt, fmt.Errorf("write learned triggers: %w", err)
	}

	return lt, nil
}

// LoadLearned loads previously generated learned triggers.
func (fs *FeedbackStore) LoadLearned() *LearnedTriggers {
	path := filepath.Join(fs.dir, "learned-triggers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return &LearnedTriggers{
			Triggers:     make(map[string][]string),
			AntiTriggers: make(map[string][]string),
		}
	}

	var lt LearnedTriggers
	if err := json.Unmarshal(data, &lt); err != nil {
		return &LearnedTriggers{
			Triggers:     make(map[string][]string),
			AntiTriggers: make(map[string][]string),
		}
	}
	if lt.Triggers == nil {
		lt.Triggers = make(map[string][]string)
	}
	if lt.AntiTriggers == nil {
		lt.AntiTriggers = make(map[string][]string)
	}
	return &lt
}

// mergeTokens adds new tokens to existing, deduplicating and capping at maxTokens.
func mergeTokens(existing, newTokens []string, maxTokens int) []string {
	seen := make(map[string]bool, len(existing))
	for _, t := range existing {
		seen[t] = true
	}
	result := append([]string{}, existing...)
	for _, t := range newTokens {
		if len(result) >= maxTokens {
			break
		}
		if !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}

func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:8])
}
