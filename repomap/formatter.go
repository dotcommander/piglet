package repomap

import (
	"fmt"
	"strings"
)

// FormatMap formats ranked files into a token-budgeted text representation.
// maxTokens controls the output size (estimated as len(text)/4).
// Returns empty string if no files have symbols.
func FormatMap(files []RankedFile, maxTokens int) string {
	totalFiles, totalSymbols := countTotals(files)
	if totalFiles == 0 {
		return ""
	}

	var b strings.Builder

	header := fmt.Sprintf("## Repository Map (%d files, %d symbols)\n\n", totalFiles, totalSymbols)
	fmt.Fprint(&b, header)

	shownFiles := 0
	shownSymbols := 0
	budgetBytes := maxTokens * 4

	for _, f := range files {
		if len(f.Symbols) == 0 {
			continue
		}

		block := formatFileBlock(f)

		// Always include at least the first file; truncate its symbols if needed.
		if shownFiles == 0 {
			remaining := budgetBytes - b.Len()
			if len(block) > remaining {
				block = truncateFileBlock(f, remaining)
			}
		} else if b.Len()+len(block) > budgetBytes {
			break
		}

		fmt.Fprint(&b, block)
		shownFiles++
		shownSymbols += len(f.Symbols)
	}

	if shownFiles < totalFiles || shownSymbols < totalSymbols {
		fmt.Fprintf(&b, "(%d symbols across %d files, showing top %d)\n", totalSymbols, totalFiles, shownFiles)
	}

	return b.String()
}

// countTotals returns the count of files with symbols and the total symbol count.
func countTotals(files []RankedFile) (int, int) {
	nFiles := 0
	nSymbols := 0
	for _, f := range files {
		if len(f.Symbols) > 0 {
			nFiles++
			nSymbols += len(f.Symbols)
		}
	}
	return nFiles, nSymbols
}

// formatFileBlock returns the full formatted block for a single file.
func formatFileBlock(f RankedFile) string {
	var b strings.Builder
	fmt.Fprint(&b, formatFileLine(f))
	for _, s := range f.Symbols {
		fmt.Fprintf(&b, "  %s\n", formatSymbol(s))
	}
	fmt.Fprint(&b, "\n")
	return b.String()
}

// formatFileLine returns the header line for a file block (path + tag annotation).
func formatFileLine(f RankedFile) string {
	switch {
	case f.Tag == "entry":
		return fmt.Sprintf("%s [entry]\n", f.Path)
	case f.Score > 0:
		return fmt.Sprintf("%s [%d refs]\n", f.Path, f.Score)
	default:
		return fmt.Sprintf("%s\n", f.Path)
	}
}

// formatSymbol formats a single symbol into its display string.
func formatSymbol(s Symbol) string {
	switch s.Kind {
	case "function":
		return formatFunc(s)
	case "method":
		return formatMethod(s)
	case "struct":
		return fmt.Sprintf("type %s struct", s.Name)
	case "interface":
		return fmt.Sprintf("type %s interface", s.Name)
	case "type":
		// Generic type alias or defined type — include signature if it carries extra info.
		if s.Signature != "" && s.Signature != s.Name {
			return fmt.Sprintf("type %s %s", s.Name, s.Signature)
		}
		return fmt.Sprintf("type %s", s.Name)
	case "enum":
		return fmt.Sprintf("type %s enum", s.Name)
	case "constant":
		return fmt.Sprintf("const %s", s.Name)
	case "variable":
		return fmt.Sprintf("var %s", s.Name)
	case "class":
		return fmt.Sprintf("class %s", s.Name)
	default:
		// Language-specific fallback using Signature when present.
		if s.Signature != "" {
			return s.Signature
		}
		return s.Name
	}
}

// formatFunc formats a function symbol.
// Signature holds "params + return" without the "func" keyword.
func formatFunc(s Symbol) string {
	if s.Signature != "" {
		return fmt.Sprintf("func %s%s", s.Name, s.Signature)
	}
	return fmt.Sprintf("func %s()", s.Name)
}

// formatMethod formats a method symbol.
// Signature holds "params + return" without the "func" keyword.
func formatMethod(s Symbol) string {
	sig := s.Signature
	if sig == "" {
		sig = "()"
	}
	if s.Receiver != "" {
		return fmt.Sprintf("func (%s) %s%s", s.Receiver, s.Name, sig)
	}
	return fmt.Sprintf("func %s%s", s.Name, sig)
}

// truncateFileBlock formats the file header and as many symbols as fit within byteLimit.
func truncateFileBlock(f RankedFile, byteLimit int) string {
	var b strings.Builder
	header := formatFileLine(f)
	fmt.Fprint(&b, header)

	for _, s := range f.Symbols {
		line := fmt.Sprintf("  %s\n", formatSymbol(s))
		if b.Len()+len(line) > byteLimit {
			break
		}
		fmt.Fprint(&b, line)
	}
	fmt.Fprint(&b, "\n")
	return b.String()
}
