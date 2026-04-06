package shell

import (
	"slices"
	"strings"
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
