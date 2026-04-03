package route

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DomainExtractor detects technology domains from prompt text and project context.
type DomainExtractor struct {
	domains map[string]DomainDef
}

// NewDomainExtractor creates an extractor from the domain taxonomy.
func NewDomainExtractor(cfg DomainsConfig) *DomainExtractor {
	return &DomainExtractor{domains: cfg.Domains}
}

// Extract returns all detected domains from prompt text and optional project dir.
func (de *DomainExtractor) Extract(text, projectDir string) []string {
	var detected []string
	seen := make(map[string]bool)

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			detected = append(detected, name)
		}
	}

	lower := strings.ToLower(text)

	// Source 1: Prompt keyword matches
	for name, def := range de.domains {
		for _, kw := range def.Keywords {
			if strings.Contains(lower, kw) {
				add(name)
				break
			}
		}
	}

	// Source 2: File references in prompt (backtick-quoted or bare filenames)
	for _, ref := range extractFileRefs(text) {
		ext := strings.ToLower(filepath.Ext(ref))
		if ext == "" {
			continue
		}
		for name, def := range de.domains {
			for _, dext := range def.Extensions {
				if ext == dext {
					add(name)
				}
			}
		}
	}

	// Source 3: Project file markers (if project dir provided)
	if projectDir != "" {
		for name, def := range de.domains {
			for _, marker := range def.Projects {
				if marker == "" {
					continue
				}
				path := filepath.Join(projectDir, marker)
				if _, err := os.Stat(path); err == nil {
					add(name)
					break
				}
			}
		}
	}

	return detected
}

// fileRefPattern matches backtick-quoted filenames or bare path-like references.
var fileRefPattern = regexp.MustCompile("`([^`]+\\.[a-zA-Z0-9]+)`|\\b([a-zA-Z0-9_/.-]+\\.[a-zA-Z]{1,6})\\b")

func extractFileRefs(text string) []string {
	matches := fileRefPattern.FindAllStringSubmatch(text, -1)
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		if m[1] != "" {
			refs = append(refs, m[1])
		} else if m[2] != "" {
			refs = append(refs, m[2])
		}
	}
	return refs
}
