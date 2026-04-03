package sift

import (
	"github.com/dotcommander/piglet/extensions/internal/xdg"
)

type CompressionConfig struct {
	CollapseBlankLines      int    `yaml:"collapse_blank_lines"`
	CollapseRepeatedLines   int    `yaml:"collapse_repeated_lines"`
	StripTrailingWhitespace bool   `yaml:"strip_trailing_whitespace"`
	TruncationMarker        string `yaml:"truncation_marker"`
}

type StructuredRule struct {
	Tool    string   `yaml:"tool"`
	Detect  string   `yaml:"detect"`
	Columns []string `yaml:"columns"`
	MaxRows int      `yaml:"max_rows"`
	SortBy  string   `yaml:"sort_by"`
}

type Config struct {
	Enabled       bool              `yaml:"enabled"`
	SizeThreshold int               `yaml:"size_threshold"`
	MaxSize       int               `yaml:"max_size"`
	Tools         []string          `yaml:"tools"`
	Compression   CompressionConfig `yaml:"compression"`
	Structured    []StructuredRule  `yaml:"structured"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		SizeThreshold: 4096,
		MaxSize:       32768,
		Tools:         []string{"Read", "Grep", "Bash"},
		Compression: CompressionConfig{
			CollapseBlankLines:      3,
			CollapseRepeatedLines:   5,
			StripTrailingWhitespace: true,
			TruncationMarker:        "\n[SIFT: truncated — {kept}/{total} bytes shown]",
		},
		Structured: []StructuredRule{
			{
				Tool:    "Bash",
				Detect:  "golangci-lint",
				Columns: []string{"file", "line", "linter", "message"},
				MaxRows: 25,
			},
			{
				Tool:    "Bash",
				Detect:  "go vet",
				Columns: []string{"file", "line", "message"},
				MaxRows: 25,
			},
			{
				Tool:    "Bash",
				Detect:  "staticcheck",
				Columns: []string{"file", "line", "message"},
				MaxRows: 25,
			},
		},
	}
}

func LoadConfig() Config {
	return xdg.LoadYAMLExt("sift", "sift.yaml", DefaultConfig())
}
