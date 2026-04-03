package depgraph

import (
	"sort"
	"strings"
)

// DepEntry is a single node in a dependency traversal result.
type DepEntry struct {
	ImportPath string `json:"import_path"`
	Depth      int    `json:"depth"`
}

// Deps returns all packages that pkg depends on (transitively), up to maxDepth.
// If maxDepth <= 0, no depth limit.
func (g *Graph) Deps(pkg string, maxDepth int) []DepEntry {
	return bfsEntries(pkg, maxDepth, g.Forward)
}

// ReverseDeps returns all packages that depend on pkg (transitively), up to maxDepth.
func (g *Graph) ReverseDeps(pkg string, maxDepth int) []DepEntry {
	return bfsEntries(pkg, maxDepth, g.Reverse)
}

// bfsEntries performs a BFS from start through edges, respecting maxDepth.
func bfsEntries(start string, maxDepth int, edges map[string][]string) []DepEntry {
	type node struct {
		pkg   string
		depth int
	}

	visited := map[string]bool{start: true}
	queue := []node{{start, 0}}
	var results []DepEntry

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, neighbor := range edges[cur.pkg] {
			if visited[neighbor] {
				continue
			}
			visited[neighbor] = true
			nextDepth := cur.depth + 1
			results = append(results, DepEntry{ImportPath: neighbor, Depth: nextDepth})
			if maxDepth <= 0 || nextDepth < maxDepth {
				queue = append(queue, node{neighbor, nextDepth})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Depth != results[j].Depth {
			return results[i].Depth < results[j].Depth
		}
		return results[i].ImportPath < results[j].ImportPath
	})

	return results
}

// Impact returns all packages affected by changes to the given file or package.
func (g *Graph) Impact(input string) []string {
	pkg, ok := g.ResolvePackage(input)
	if !ok {
		return nil
	}

	entries := g.ReverseDeps(pkg, 0)
	affected := make([]string, 0, len(entries)+1)
	affected = append(affected, pkg)
	for _, e := range entries {
		affected = append(affected, e.ImportPath)
	}

	sort.Strings(affected)
	return affected
}

// Cycle represents a circular dependency chain.
type Cycle struct {
	Path []string `json:"path"`
}

// DetectCycles finds all circular dependencies in the graph using DFS coloring.
func (g *Graph) DetectCycles() []Cycle {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(g.Packages))
	stack := []string{}
	seen := map[string]bool{}
	var cycles []Cycle

	var dfs func(pkg string)
	dfs = func(pkg string) {
		color[pkg] = gray
		stack = append(stack, pkg)

		neighbors := g.Forward[pkg]

		for _, neighbor := range neighbors {
			if color[neighbor] == gray {
				// Found a back edge — extract cycle.
				start := -1
				for i, p := range stack {
					if p == neighbor {
						start = i
						break
					}
				}
				if start >= 0 {
					raw := make([]string, len(stack)-start)
					copy(raw, stack[start:])
					normalized := normalizeCycle(raw)
					key := cycleKey(normalized)
					if !seen[key] {
						seen[key] = true
						cycles = append(cycles, Cycle{Path: normalized})
					}
				}
			} else if color[neighbor] == white {
				dfs(neighbor)
			}
		}

		stack = stack[:len(stack)-1]
		color[pkg] = black
	}

	// Iterate packages in deterministic order.
	pkgs := make([]string, 0, len(g.Packages))
	for p := range g.Packages {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	for _, p := range pkgs {
		if color[p] == white {
			dfs(p)
		}
	}

	sort.Slice(cycles, func(i, j int) bool {
		return cycleKey(cycles[i].Path) < cycleKey(cycles[j].Path)
	})

	return cycles
}

// normalizeCycle rotates path so it starts with the lexicographically smallest element.
func normalizeCycle(path []string) []string {
	if len(path) == 0 {
		return path
	}
	minIdx := 0
	for i := 1; i < len(path); i++ {
		if path[i] < path[minIdx] {
			minIdx = i
		}
	}
	rotated := make([]string, len(path))
	copy(rotated, path[minIdx:])
	copy(rotated[len(path)-minIdx:], path[:minIdx])
	return rotated
}

func cycleKey(path []string) string {
	return strings.Join(path, "\x00")
}

// ShortestPath finds the shortest dependency path from src to dst via Forward edges.
// Returns nil if no path exists.
func (g *Graph) ShortestPath(src, dst string) []string {
	if src == dst {
		return []string{src}
	}

	parent := map[string]string{src: ""}
	queue := []string{src}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		neighbors := g.Forward[cur]

		for _, neighbor := range neighbors {
			if _, visited := parent[neighbor]; visited {
				continue
			}
			parent[neighbor] = cur
			if neighbor == dst {
				return reconstructPath(parent, src, dst)
			}
			queue = append(queue, neighbor)
		}
	}

	return nil
}

func reconstructPath(parent map[string]string, src, dst string) []string {
	var path []string
	for cur := dst; cur != src; cur = parent[cur] {
		path = append(path, cur)
	}
	path = append(path, src)
	// Reverse.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
