package shell

import (
	"fmt"
	"strings"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// bgEntry tracks a single named background task.
type bgEntry struct {
	agent   *core.Agent
	eventCh <-chan core.Event
	prompt  string
	result  strings.Builder
}

// startBackground is the callback wired into ext.App via WithRunBackground.
// Uses the prompt as the task name (truncated to 20 runes).
func (s *Shell) startBackground(prompt string) error {
	name := TruncateRunes(prompt, 20)
	return s.StartBackgroundNamed(name, prompt)
}

// StartBackgroundNamed starts a named background agent with the given prompt.
func (s *Shell) StartBackgroundNamed(name, prompt string) error {
	s.mu.Lock()
	if s.bgTasks == nil {
		s.bgTasks = make(map[string]*bgEntry)
	}
	if _, exists := s.bgTasks[name]; exists {
		s.mu.Unlock()
		return fmt.Errorf("background task %q already running", name)
	}
	agent := s.agent
	s.mu.Unlock()

	if agent == nil {
		return fmt.Errorf("agent not ready")
	}

	tools := s.app.BackgroundSafeTools()
	bgMax := 5
	if s.settings != nil {
		bgMax = config.IntOr(s.settings.Agent.BgMaxTurns, 5)
	}

	bgAgent := core.NewAgent(core.AgentConfig{
		System:   agent.System(),
		Provider: agent.Provider(),
		Tools:    tools,
		MaxTurns: bgMax,
	})

	ch := bgAgent.Start(s.ctx, prompt)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.bgTasks[name] = &bgEntry{
		agent:   bgAgent,
		eventCh: ch,
		prompt:  prompt,
	}

	s.notifications = append(s.notifications, Notification{
		Kind: NotifyStatus,
		Key:  ext.StatusKeyBg,
		Text: "bg: " + name,
	})

	return nil
}

// StopBackground cancels all running background tasks.
func (s *Shell) StopBackground() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, entry := range s.bgTasks {
		if entry.agent != nil && entry.agent.IsRunning() {
			entry.agent.Stop()
		}
		delete(s.bgTasks, name)
	}
	s.notifications = append(s.notifications, Notification{
		Kind: NotifyStatus,
		Key:  ext.StatusKeyBg,
		Text: "",
	})
}

// StopBackgroundNamed cancels a specific named background task.
func (s *Shell) StopBackgroundNamed(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.bgTasks[name]
	if !ok {
		return
	}
	if entry.agent != nil && entry.agent.IsRunning() {
		entry.agent.Stop()
	}
	delete(s.bgTasks, name)
	if len(s.bgTasks) == 0 {
		s.notifications = append(s.notifications, Notification{
			Kind: NotifyStatus,
			Key:  ext.StatusKeyBg,
			Text: "",
		})
	}
}

// isBackgroundRunning returns whether any background task is currently active.
func (s *Shell) isBackgroundRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.bgTasks {
		if entry.agent != nil && entry.agent.IsRunning() {
			return true
		}
	}
	return false
}

// BackgroundTasks returns the names of all active background tasks.
func (s *Shell) BackgroundTasks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.bgTasks))
	for name := range s.bgTasks {
		names = append(names, name)
	}
	return names
}

// TruncateRunes truncates s to max runes, appending "…" (U+2026) if truncated.
func TruncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
