package memory

import "slices"

const maxGraphDepth = 10

// Related returns all facts reachable from startKey via relation edges,
// up to maxDepth hops. Returns empty slice if startKey doesn't exist.
// maxDepth <= 0 means unlimited (capped at maxGraphDepth to prevent runaway).
// The startKey itself is excluded from results.
// Results are sorted by key for deterministic output.
func Related(s *Store, startKey string, maxDepth int) []Fact {
	if maxDepth <= 0 || maxDepth > maxGraphDepth {
		maxDepth = maxGraphDepth
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.data[startKey]; !ok {
		return nil
	}

	type entry struct {
		key   string
		depth int
	}

	visited := map[string]bool{startKey: true}
	queue := []entry{{key: startKey, depth: 0}}
	var results []Fact

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if cur.depth >= maxDepth {
			continue
		}

		fact, ok := s.data[cur.key]
		if !ok {
			continue
		}

		for _, rel := range fact.Relations {
			if visited[rel] {
				continue
			}
			visited[rel] = true
			if f, exists := s.data[rel]; exists {
				results = append(results, f)
				queue = append(queue, entry{key: rel, depth: cur.depth + 1})
			}
		}
	}

	slices.SortFunc(results, func(a, b Fact) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})

	return results
}
