package shell

import (
	"slices"
	"strings"
	"time"

	"github.com/dotcommander/piglet/core"
)

// QueueMode controls how the shell drains its input queue on EventAgentEnd.
type QueueMode int

const (
	// QueueDrainAll processes all queued items at once, merging prompts
	// into a single turn. This is the default.
	QueueDrainAll QueueMode = iota

	// QueueSingleStep processes one item from the queue per agent turn.
	// Remaining items stay queued for the next EventAgentEnd.
	QueueSingleStep
)

type queuePriority int

const (
	priorityNext  queuePriority = iota // user prompts — send ASAP
	priorityLater                      // slash commands — defer until idle
)

const maxQueueSize = 50

type queuedItem struct {
	content  string
	priority queuePriority
}

func drainQueue(q *[]queuedItem) []queuedItem {
	items := slices.Clone(*q)
	*q = (*q)[:0]
	slices.SortFunc(items, func(a, b queuedItem) int {
		return int(a.priority) - int(b.priority)
	})
	return items
}

// drainOne removes and returns the highest-priority item from the queue.
// Returns nil if the queue is empty.
func drainOne(q *[]queuedItem) *queuedItem {
	if len(*q) == 0 {
		return nil
	}
	// Find highest priority (lowest value)
	best := 0
	for i, it := range *q {
		if it.priority < (*q)[best].priority {
			best = i
		}
	}
	item := (*q)[best]
	*q = slices.Delete(*q, best, best+1)
	return &item
}

func drainPrompts(items []queuedItem) []queuedItem {
	return slices.DeleteFunc(slices.Clone(items), func(it queuedItem) bool {
		return it.priority == priorityLater
	})
}

func drainCommands(items []queuedItem) []queuedItem {
	return slices.DeleteFunc(slices.Clone(items), func(it queuedItem) bool {
		return it.priority != priorityLater
	})
}

func mergePrompts(items []queuedItem) string {
	contents := make([]string, 0, len(items))
	for _, it := range items {
		contents = append(contents, it.content)
	}
	return strings.Join(contents, "\n\n")
}

// drainAndSubmitQueued drains the input queue, executes queued slash commands,
// merges queued prompts into one turn, and restarts the agent if needed.
func (s *Shell) drainAndSubmitQueued() {
	s.mu.Lock()
	mode := s.queueMode
	s.mu.Unlock()

	if mode == QueueSingleStep {
		s.drainAndSubmitOne()
		return
	}

	s.mu.Lock()
	items := drainQueue(&s.queue)
	s.mu.Unlock()

	if len(items) == 0 {
		// Still drain actions — EventAgentEnd handlers may have queued some
		s.drainActions()
		return
	}

	cmds := drainCommands(slices.Clone(items))
	prompts := drainPrompts(items)

	// Execute queued slash commands
	for _, c := range cmds {
		name, args := parseSlashCommand(c.content)
		s.runCommand(name, args)
	}

	// Merge and submit queued prompts as one turn
	if len(prompts) > 0 {
		content := mergePrompts(prompts)
		userMsg := &core.UserMessage{Content: content, Timestamp: time.Now()}
		s.persistMessage(userMsg)
		s.notify(Notification{Kind: NotifyQueuedSubmit, Text: content})
		s.startAgent(content)
	}

	s.drainActions()
}

// drainAndSubmitOne processes a single item from the queue.
func (s *Shell) drainAndSubmitOne() {
	s.mu.Lock()
	item := drainOne(&s.queue)
	s.mu.Unlock()

	if item == nil {
		s.drainActions()
		return
	}

	if item.priority == priorityLater {
		name, args := parseSlashCommand(item.content)
		s.runCommand(name, args)
	} else {
		userMsg := &core.UserMessage{Content: item.content, Timestamp: time.Now()}
		s.persistMessage(userMsg)
		s.notify(Notification{Kind: NotifyQueuedSubmit, Text: item.content})
		s.startAgent(item.content)
	}

	s.drainActions()
}

// drainPromptQueue returns only non-command items from the queue (mid-turn steering).
func (s *Shell) drainPromptQueue() []queuedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	var prompts []queuedItem
	j := 0
	for _, it := range s.queue {
		if it.priority == priorityLater {
			s.queue[j] = it
			j++
		} else {
			prompts = append(prompts, it)
		}
	}
	s.queue = s.queue[:j]
	return prompts
}
