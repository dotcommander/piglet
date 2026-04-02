package shell

import (
	"slices"
	"strings"
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
