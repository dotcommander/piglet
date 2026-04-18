package repomap

import (
	"os"
	"path/filepath"
	"strings"
)

// maxScanLines is the maximum number of lines scanned per file.
const maxScanLines = 500

// ParseGenericFile extracts symbols from a non-Go source file using regex
// patterns. path is absolute, root is the project root for relative path
// calculation.
func ParseGenericFile(path, root, language string) (*FileSymbols, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}

	fs := &FileSymbols{
		Path:     rel,
		Language: language,
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > maxScanLines {
		lines = lines[:maxScanLines]
	}

	switch language {
	case "typescript", "javascript", "tsx", "jsx":
		parseTS(lines, fs)
	case "python":
		parsePython(lines, fs)
	case "rust":
		parseRust(lines, fs)
	case "c", "cpp", "c++", "cxx":
		parseC(lines, fs)
	case "java":
		parseJava(lines, fs)
	case "ruby":
		parseRuby(lines, fs)
	case "php":
		parsePHP(lines, fs)
		// swift, kotlin, lua, zig — unsupported, return empty
	}

	return fs, nil
}

// trackBlockComment advances the block-comment state machine for C-family
// languages (/* ... */). It returns the new inBlockComment state.
func trackBlockComment(trimmed string, inBlockComment bool) bool {
	if inBlockComment {
		if idx := strings.Index(trimmed, "*/"); idx >= 0 {
			return false
		}
		return true
	}
	if idx := strings.Index(trimmed, "/*"); idx >= 0 {
		// Only enter block comment if the closing */ is not on the same line.
		rest := trimmed[idx+2:]
		if !strings.Contains(rest, "*/") {
			return true
		}
	}
	return false
}
