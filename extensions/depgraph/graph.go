package depgraph

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Package holds metadata for one Go package in the graph.
type Package struct {
	ImportPath string   `json:"import_path"`
	Dir        string   `json:"dir"`
	Name       string   `json:"name"`
	Imports    []string `json:"imports,omitempty"`
}

// Graph holds the complete dependency graph for a Go module.
type Graph struct {
	Root     string              // module root directory
	Module   string              // module path (e.g. "github.com/foo/bar")
	Packages map[string]*Package // import path → package
	Forward  map[string][]string // import path → packages it imports (within module)
	Reverse  map[string][]string // import path → packages that import it (within module)
	dirIndex map[string]string   // dir → import path, lazy-initialized from Packages
}

type goListPkg struct {
	ImportPath   string   `json:"ImportPath"`
	Dir          string   `json:"Dir"`
	Name         string   `json:"Name"`
	GoFiles      []string `json:"GoFiles"`
	Imports      []string `json:"Imports"`
	TestImports  []string `json:"TestImports"`
	XTestImports []string `json:"XTestImports"`
}

// BuildGraph scans the Go module at root and builds a complete dependency graph.
func BuildGraph(root string) (*Graph, error) {
	mod, err := readModulePath(root)
	if err != nil {
		return nil, err
	}

	raw, err := runGoList(root)
	if err != nil {
		return nil, err
	}

	var entries []goListPkg
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var e goListPkg
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("go list parse: %w", err)
		}
		entries = append(entries, e)
	}

	packages := make(map[string]*Package, len(entries))
	for _, e := range entries {
		packages[e.ImportPath] = &Package{
			ImportPath: e.ImportPath,
			Dir:        e.Dir,
			Name:       e.Name,
		}
	}

	forward := make(map[string][]string, len(entries))
	for _, e := range entries {
		seen := make(map[string]struct{})
		all := make([]string, 0, len(e.Imports)+len(e.TestImports)+len(e.XTestImports))
		all = append(all, e.Imports...)
		all = append(all, e.TestImports...)
		all = append(all, e.XTestImports...)

		for _, imp := range all {
			if imp == e.ImportPath {
				continue // skip self-imports (xtest → own package)
			}
			if _, dup := seen[imp]; dup {
				continue
			}
			if _, inModule := packages[imp]; !inModule {
				continue
			}
			seen[imp] = struct{}{}
			forward[e.ImportPath] = append(forward[e.ImportPath], imp)
		}

		deps := forward[e.ImportPath]
		sort.Strings(deps)
		forward[e.ImportPath] = deps

		packages[e.ImportPath].Imports = deps
	}

	reverse := make(map[string][]string, len(entries))
	for pkg, deps := range forward {
		for _, dep := range deps {
			reverse[dep] = append(reverse[dep], pkg)
		}
	}
	for dep := range reverse {
		sort.Strings(reverse[dep])
	}

	dirIndex := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Dir != "" {
			dirIndex[e.Dir] = e.ImportPath
		}
	}

	return &Graph{
		Root:     root,
		Module:   mod,
		Packages: packages,
		Forward:  forward,
		Reverse:  reverse,
		dirIndex: dirIndex,
	}, nil
}

// ResolvePackage finds the package import path for a given file path or package pattern.
// Accepts: full import path, relative path from root, or a file path.
func (g *Graph) ResolvePackage(input string) (string, bool) {
	if _, ok := g.Packages[input]; ok {
		return input, true
	}

	if rel, ok := strings.CutPrefix(input, "./"); ok {
		candidate := strings.TrimSuffix(g.Module+"/"+rel, "/")
		if _, ok := g.Packages[candidate]; ok {
			return candidate, true
		}
	}

	abs := input
	if !filepath.IsAbs(input) {
		abs = filepath.Join(g.Root, input)
	}
	abs = filepath.Clean(abs)
	g.ensureDirIndex()

	// Exact dir match first.
	if ip, ok := g.dirIndex[abs]; ok {
		return ip, true
	}
	// File inside a package dir — walk up until we find a package.
	dir := filepath.Dir(abs)
	for {
		if ip, ok := g.dirIndex[dir]; ok {
			return ip, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", false
}

func (g *Graph) ensureDirIndex() {
	if g.dirIndex != nil {
		return
	}
	g.dirIndex = make(map[string]string, len(g.Packages))
	for ip, pkg := range g.Packages {
		if pkg.Dir != "" {
			g.dirIndex[pkg.Dir] = ip
		}
	}
}

func readModulePath(root string) (_ string, retErr error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if mod, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(mod), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

func runGoList(root string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	return out, nil
}
