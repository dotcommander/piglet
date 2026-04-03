package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotcommander/piglet/extensions/depgraph"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	depth := fs.Int("depth", 0, "max traversal depth (0=unlimited)")
	tokens := fs.Int("tokens", 0, "token budget (0=unlimited)")
	jsonOut := fs.Bool("json", false, "JSON output")
	noCache := fs.Bool("no-cache", false, "force rebuild, ignore cache")
	fs.Parse(os.Args[2:])

	// Build graph from current directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getwd: %v\n", err)
		os.Exit(2)
	}

	g, err := buildGraph(cwd, *noCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	switch cmd {
	case "deps":
		if fs.NArg() < 1 {
			usage()
			os.Exit(2)
		}
		pkg, ok := g.ResolvePackage(fs.Arg(0))
		if !ok {
			fmt.Fprintf(os.Stderr, "package not found: %s\n", fs.Arg(0))
			os.Exit(1)
		}
		entries := g.Deps(pkg, *depth)
		if *jsonOut {
			printJSON(entries)
		} else {
			fmt.Println(depgraph.FormatDeps(pkg, g.Module, entries, *tokens))
		}

	case "rdeps":
		if fs.NArg() < 1 {
			usage()
			os.Exit(2)
		}
		pkg, ok := g.ResolvePackage(fs.Arg(0))
		if !ok {
			fmt.Fprintf(os.Stderr, "package not found: %s\n", fs.Arg(0))
			os.Exit(1)
		}
		entries := g.ReverseDeps(pkg, *depth)
		if *jsonOut {
			printJSON(entries)
		} else {
			fmt.Println(depgraph.FormatDeps(pkg, g.Module, entries, *tokens))
		}

	case "impact":
		if fs.NArg() < 1 {
			usage()
			os.Exit(2)
		}
		packages := g.Impact(fs.Arg(0))
		if *jsonOut {
			printJSON(packages)
		} else {
			fmt.Println(depgraph.FormatImpact(packages, g.Module, *tokens))
		}

	case "cycles":
		cycles := g.DetectCycles()
		if *jsonOut {
			printJSON(cycles)
		} else {
			fmt.Println(depgraph.FormatCycles(cycles, g.Module))
		}

	case "path":
		if fs.NArg() < 2 {
			usage()
			os.Exit(2)
		}
		src, ok1 := g.ResolvePackage(fs.Arg(0))
		dst, ok2 := g.ResolvePackage(fs.Arg(1))
		if !ok1 {
			fmt.Fprintf(os.Stderr, "package not found: %s\n", fs.Arg(0))
			os.Exit(1)
		}
		if !ok2 {
			fmt.Fprintf(os.Stderr, "package not found: %s\n", fs.Arg(1))
			os.Exit(1)
		}
		p := g.ShortestPath(src, dst)
		if *jsonOut {
			printJSON(p)
		} else {
			fmt.Println(depgraph.FormatPath(p, g.Module))
		}

	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: depgraph <command> [flags] [args]

Commands:
  deps <package>        Dependencies (what does this import?)
  rdeps <package>       Reverse deps (what imports this?)
  impact <file|pkg>     Blast radius of changes
  cycles                Detect circular dependencies
  path <from> <to>      Shortest dependency path

Flags:
  -depth int      Max traversal depth (0=unlimited)
  -tokens int     Token budget (0=unlimited)
  -json           JSON output
  -no-cache       Force rebuild, ignore cache
`)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error: json encode: %v\n", err)
		os.Exit(1)
	}
}

// buildGraph loads from cache if available, otherwise builds and caches.
func buildGraph(root string, noCache bool) (*depgraph.Graph, error) {
	if !noCache {
		if g, err := loadCache(root); err == nil {
			return g, nil
		}
	}

	g, err := depgraph.BuildGraph(root)
	if err != nil {
		return nil, err
	}

	// Best-effort cache write — failure is non-fatal.
	_ = saveCache(root, g)
	return g, nil
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "depgraph"), nil
}

func cacheKey(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func cacheFilePath(root string) (string, error) {
	key, err := cacheKey(root)
	if err != nil {
		return "", err
	}
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+".json"), nil
}

func loadCache(root string) (*depgraph.Graph, error) {
	path, err := cacheFilePath(root)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var g depgraph.Graph
	if err := json.NewDecoder(f).Decode(&g); err != nil {
		return nil, err
	}
	return &g, nil
}

func saveCache(root string, g *depgraph.Graph) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path, err := cacheFilePath(root)
	if err != nil {
		return err
	}

	// Atomic write: temp file then rename.
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(g); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
