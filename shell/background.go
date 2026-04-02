package shell

import (
	"fmt"

	"github.com/dotcommander/piglet/config"
	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// startBackground is the callback wired into ext.App via WithRunBackground.
func (s *Shell) startBackground(prompt string) error {
	s.mu.Lock()
	if s.bgAgent != nil && s.bgAgent.IsRunning() {
		s.mu.Unlock()
		return fmt.Errorf("background task already running")
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

	s.mu.Lock()
	defer s.mu.Unlock()

	s.bgAgent = core.NewAgent(core.AgentConfig{
		System:   agent.System(),
		Provider: agent.Provider(),
		Tools:    tools,
		MaxTurns: bgMax,
	})

	ch := s.bgAgent.Start(s.ctx, prompt)
	s.bgEventCh = ch
	s.bgTask = prompt
	s.bgResult.Reset()

	task := TruncateRunes(prompt, 20)
	s.notifications = append(s.notifications, Notification{
		Kind: NotifyStatus,
		Key:  ext.StatusKeyBg,
		Text: "bg: " + task,
	})

	return nil
}

// StopBackground cancels the running background agent.
func (s *Shell) StopBackground() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bgAgent != nil && s.bgAgent.IsRunning() {
		s.bgAgent.Stop()
	}
	s.bgAgent = nil
	s.bgEventCh = nil
	s.bgTask = ""
	s.bgResult.Reset()
	s.notifications = append(s.notifications, Notification{
		Kind: NotifyStatus,
		Key:  ext.StatusKeyBg,
		Text: "",
	})
}

// isBackgroundRunning returns whether a background agent is currently active.
func (s *Shell) isBackgroundRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bgAgent != nil && s.bgAgent.IsRunning()
}

// TruncateRunes truncates s to max runes, appending "…" (U+2026) if truncated.
func TruncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
