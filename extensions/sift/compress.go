package sift

import (
	"fmt"
	"strings"
)

func CompressWithTool(toolName, text string, cfg Config, cwd string) string {
	if structured, ok := CompressStructured(toolName, text, cfg, cwd); ok {
		return structured
	}
	return Compress(text, cfg)
}

func Compress(text string, cfg Config) string {
	if len(text) < cfg.SizeThreshold {
		return text
	}

	original := text

	lines := strings.Split(text, "\n")

	if cfg.Compression.StripTrailingWhitespace {
		lines = stripTrailingWhitespace(lines)
	}

	if cfg.Compression.CollapseBlankLines > 0 {
		lines = collapseBlankLines(lines, cfg.Compression.CollapseBlankLines)
	}

	if cfg.Compression.CollapseRepeatedLines > 0 {
		lines = collapseRepeatedLines(lines, cfg.Compression.CollapseRepeatedLines)
	}

	text = strings.Join(lines, "\n")

	if cfg.MaxSize > 0 {
		text = truncate(text, cfg.MaxSize, cfg.Compression.TruncationMarker)
	}

	if len(text) >= len(original) {
		return original
	}

	pct := 100 - (len(text)*100)/len(original)
	header := fmt.Sprintf("[SIFT: %d -> %d bytes (%d%% reduction)]\n", len(original), len(text), pct)
	return header + text
}

func stripTrailingWhitespace(lines []string) []string {
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return lines
}

func collapseBlankLines(lines []string, threshold int) []string {
	result := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			continue
		}
		if blankCount > 0 {
			emit := blankCount
			if blankCount >= threshold {
				emit = 1
			}
			for range emit {
				result = append(result, "")
			}
			blankCount = 0
		}
		result = append(result, line)
	}
	if blankCount > 0 {
		emit := blankCount
		if blankCount >= threshold {
			emit = 1
		}
		for range emit {
			result = append(result, "")
		}
	}
	return result
}

func collapseRepeatedLines(lines []string, threshold int) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		count := j - i
		result = append(result, lines[i])
		if count >= threshold {
			result = append(result, fmt.Sprintf("[... %d identical lines collapsed]", count-1))
		} else {
			for k := i + 1; k < j; k++ {
				result = append(result, lines[k])
			}
		}
		i = j
	}
	return result
}

func truncate(text string, maxSize int, marker string) string {
	if len(text) <= maxSize {
		return text
	}

	totalSize := len(text)

	cutoff := maxSize - len(marker)
	if cutoff <= 0 {
		cutoff = maxSize
	}

	truncated := text[:cutoff]
	lastNewline := strings.LastIndex(truncated, "\n")
	if lastNewline > 0 {
		truncated = truncated[:lastNewline]
	}

	resolved := strings.ReplaceAll(marker, "{kept}", fmt.Sprintf("%d", len(truncated)))
	resolved = strings.ReplaceAll(resolved, "{total}", fmt.Sprintf("%d", totalSize))

	return truncated + resolved
}
