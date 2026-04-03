package confirm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type goListEntry struct {
	ImportPath   string   `json:"ImportPath"`
	Dir          string   `json:"Dir"`
	GoFiles      []string `json:"GoFiles"`
	TestGoFiles  []string `json:"TestGoFiles"`
	XTestGoFiles []string `json:"XTestGoFiles"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
}

// AffectedPackages returns the deduplicated sorted list of package import paths
// that are directly or transitively affected by the given changed files.
func AffectedPackages(changedFiles []string, root string) ([]string, error) {
	if len(changedFiles) == 0 {
		return nil, nil
	}

	pkgs, err := listPackages(root)
	if err != nil {
		return nil, err
	}

	// file path → import path
	fileToImport := make(map[string]string)
	for _, p := range pkgs {
		for _, f := range allSourceFiles(p) {
			fileToImport[filepath.Join(p.Dir, f)] = p.ImportPath
		}
	}

	// import path → set of all import paths (across normal + test imports)
	knownPkgs := make(map[string]struct{}, len(pkgs))
	for _, p := range pkgs {
		knownPkgs[p.ImportPath] = struct{}{}
	}

	// reverse graph: importPath → packages that import it
	revGraph := make(map[string][]string)
	for _, p := range pkgs {
		seen := make(map[string]struct{})
		for _, dep := range allImports(p) {
			if _, ok := knownPkgs[dep]; !ok {
				continue // skip stdlib / external deps
			}
			if _, dup := seen[dep]; dup {
				continue
			}
			seen[dep] = struct{}{}
			revGraph[dep] = append(revGraph[dep], p.ImportPath)
		}
	}

	// resolve changed files → directly changed packages
	roots := make(map[string]struct{})
	for _, f := range changedFiles {
		abs := f
		if !filepath.IsAbs(f) {
			abs = filepath.Join(root, f)
		}
		abs = filepath.Clean(abs)
		if imp, ok := fileToImport[abs]; ok {
			roots[imp] = struct{}{}
		}
	}

	// BFS over reverse graph to collect all transitively affected packages
	visited := make(map[string]struct{}, len(roots))
	queue := make([]string, 0, len(roots))
	for imp := range roots {
		visited[imp] = struct{}{}
		queue = append(queue, imp)
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dependent := range revGraph[cur] {
			if _, ok := visited[dependent]; ok {
				continue
			}
			visited[dependent] = struct{}{}
			queue = append(queue, dependent)
		}
	}

	result := make([]string, 0, len(visited))
	for imp := range visited {
		result = append(result, imp)
	}
	sort.Strings(result)
	return result, nil
}

func listPackages(root string) ([]goListEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var pkgs []goListEntry
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var e goListEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("go list parse: %w", err)
		}
		pkgs = append(pkgs, e)
	}
	return pkgs, nil
}

func allSourceFiles(p goListEntry) []string {
	out := make([]string, 0, len(p.GoFiles)+len(p.TestGoFiles)+len(p.XTestGoFiles))
	out = append(out, p.GoFiles...)
	out = append(out, p.TestGoFiles...)
	out = append(out, p.XTestGoFiles...)
	return out
}

func allImports(p goListEntry) []string {
	out := make([]string, 0, len(p.Imports)+len(p.TestImports)+len(p.XTestImports))
	out = append(out, p.Imports...)
	out = append(out, p.TestImports...)
	out = append(out, p.XTestImports...)
	return out
}
