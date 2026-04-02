package shell

import (
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// Submit processes user input. It handles:
//   - Slash command dispatch (returns ResponseCommand)
//   - Input transformer pipeline
//   - Message hook execution
//   - Session persistence of user message
//   - Agent start (returns ResponseAgentStarted with event channel)
//   - Queueing if agent is already running (returns ResponseQueued)
//   - Agent-not-ready guard (returns ResponseNotReady)
func (s *Shell) Submit(input string) Response {
	return s.submitInternal(input, nil)
}

// SubmitWithImage is like Submit but attaches an image to the user message.
func (s *Shell) SubmitWithImage(input string, img *core.ImageContent) Response {
	return s.submitInternal(input, img)
}

func (s *Shell) submitInternal(input string, img *core.ImageContent) Response {
	s.mu.Lock()
	running := s.running
	agent := s.agent
	s.mu.Unlock()

	// While agent is running, queue or run immediate commands
	if running {
		if strings.HasPrefix(input, "/") {
			name, args := parseSlashCommand(input)
			if cmd := s.lookupCommand(name); cmd != nil && cmd.Immediate {
				return s.runCommand(name, args)
			}
		}
		s.enqueue(input, strings.HasPrefix(input, "/"))
		return Response{Kind: ResponseQueued}
	}

	// Slash command?
	if strings.HasPrefix(input, "/") {
		name, args := parseSlashCommand(input)
		return s.runCommand(name, args)
	}

	// Agent not ready?
	if agent == nil {
		return Response{Kind: ResponseNotReady}
	}

	// Run input transformers
	if s.app != nil {
		transformed, handled, err := s.app.RunInputTransformers(s.ctx, input)
		if err != nil {
			return Response{Kind: ResponseError, Error: err}
		}
		if handled {
			return Response{Kind: ResponseHandled}
		}
		input = transformed
	}

	// Build user message
	userMsg := &core.UserMessage{
		Content:   input,
		Timestamp: time.Now(),
	}
	if img != nil {
		userMsg.Blocks = append(userMsg.Blocks, *img)
	}
	s.persistMessage(userMsg)

	return s.startAgent(input)
}

// startAgent runs message hooks and starts the agent, returning a Response
// with the event channel.
func (s *Shell) startAgent(content string) Response {
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()

	if agent == nil {
		return Response{Kind: ResponseNotReady}
	}

	// Run message hooks for ephemeral turn context
	if s.app != nil {
		if injections, err := s.app.RunMessageHooks(s.ctx, content); err == nil && len(injections) > 0 {
			agent.SetTurnContext(injections)
		}
	}

	ch := agent.Start(s.ctx, content)

	if s.app != nil {
		s.app.ClearIdle()
	}

	s.mu.Lock()
	s.eventCh = ch
	s.running = true
	s.mu.Unlock()

	return Response{Kind: ResponseAgentStarted, Events: ch}
}

// runCommand dispatches a slash command to the registered handler.
func (s *Shell) runCommand(name, args string) Response {
	if s.app == nil {
		return Response{Kind: ResponseError, Error: fmt.Errorf("no extensions loaded")}
	}

	// Alias
	if name == "exit" {
		name = "quit"
	}

	cmds := s.app.Commands()
	cmd, ok := cmds[name]
	if !ok {
		s.notify(Notification{Kind: NotifyMessage, Text: "Unknown command: /" + name})
		return Response{Kind: ResponseCommand}
	}

	// Rebind callbacks before running command
	s.mu.Lock()
	agent := s.agent
	s.mu.Unlock()
	if agent != nil {
		s.app.Bind(agent, s.bgBindOpts()...)
	}

	// /clear: notify frontend to clear display
	if name == "clear" {
		s.notify(Notification{Kind: NotifyClearDisplay})
	}

	if err := cmd.Handler(args, s.app); err != nil {
		s.notify(Notification{Kind: NotifyError, Text: "Command error: " + err.Error()})
		return Response{Kind: ResponseCommand}
	}

	s.drainActions()

	return Response{Kind: ResponseCommand}
}

// lookupCommand finds a registered command by name.
func (s *Shell) lookupCommand(name string) *ext.Command {
	if s.app == nil {
		return nil
	}
	cmds := s.app.Commands()
	cmd, ok := cmds[name]
	if !ok {
		return nil
	}
	return cmd
}

// enqueue appends input to the queue.
func (s *Shell) enqueue(content string, lowPriority bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) >= maxQueueSize {
		return
	}
	priority := priorityNext
	if lowPriority {
		priority = priorityLater
	}
	s.queue = append(s.queue, queuedItem{content: content, priority: priority})
}

// parseSlashCommand splits "/name arg1 arg2" into ("name", "arg1 arg2").
func parseSlashCommand(text string) (name, args string) {
	text = strings.TrimPrefix(text, "/")
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", ""
	}
	name = parts[0]
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}
	return
}
