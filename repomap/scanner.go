package repomap

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// supportedExts maps file extensions to language IDs.
var supportedExts = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".py":   "python",
	".rs":   "rust",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".cc":   "cpp",
	".java": "java",
	".lua":  "lua",
	".zig":  "zig",
	".rb":   "ruby",
	".swift": "swift",
	".kt":   "kotlin",
}

const maxFileSize = 50_000

// skipDirs holds directory names to skip during filesystem walk.
var skipDirs = map[string]bool{
	".git":        true,
	"vendor":      true,
	"node_modules": true,
	"__pycache__": true,
	".venv":       true,
	"build":       true,
	"dist":        true,
	"target":      true,
}

// FileInfo holds a discovered file with its language.
type FileInfo struct {
	Path     string // relative to project root
	Language string // language ID
}

// ScanFiles discovers source files in the given directory.
// Uses git ls-files when available, falls back to directory walking.
func ScanFiles(ctx context.Context, root string) ([]FileInfo, error) {
	files, err := scanGit(ctx, root)
	if err != nil {
		files, err = scanWalk(ctx, root)
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// LanguageFor returns the language ID for a file extension, or "" if unsupported.
func LanguageFor(ext string) string {
	return supportedExts[ext]
}

func scanGit(ctx context.Context, root string) ([]FileInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lang := LanguageFor(filepath.Ext(line))
		if lang == "" {
			continue
		}

		absPath := filepath.Join(root, line)
		if tooBig(absPath) {
			continue
		}

		files = append(files, FileInfo{Path: line, Language: lang})
	}

	return files, nil
}

func scanWalk(ctx context.Context, root string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		lang := LanguageFor(filepath.Ext(path))
		if lang == "" {
			return nil
		}

		if tooBig(path) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil //nolint:nilerr // skip if relative path can't be computed
		}

		files = append(files, FileInfo{Path: rel, Language: lang})
		return nil
	})

	return files, err
}

func tooBig(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return info.Size() > maxFileSize
}
