package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		args []string
		want string
	}{
		{
			name: "no placeholders",
			body: "hello world",
			args: []string{"foo"},
			want: "hello world",
		},
		{
			name: "dollar-at all args",
			body: "Review: $@",
			args: []string{"fix", "the", "bug"},
			want: "Review: fix the bug",
		},
		{
			name: "dollar-at no args",
			body: "Review: $@",
			args: nil,
			want: "Review: ",
		},
		{
			name: "positional args",
			body: "File: $1, Line: $2",
			args: []string{"main.go", "42"},
			want: "File: main.go, Line: 42",
		},
		{
			name: "missing positional args",
			body: "A=$1 B=$2 C=$3",
			args: []string{"only"},
			want: "A=only B= C=",
		},
		{
			name: "slice from N",
			body: "rest: ${@:2}",
			args: []string{"first", "second", "third"},
			want: "rest: second third",
		},
		{
			name: "slice from N with length",
			body: "mid: ${@:2:1}",
			args: []string{"a", "b", "c"},
			want: "mid: b",
		},
		{
			name: "slice from N beyond bounds",
			body: "rest: ${@:5}",
			args: []string{"a", "b"},
			want: "rest: ",
		},
		{
			name: "slice with length exceeding bounds",
			body: "part: ${@:2:10}",
			args: []string{"a", "b", "c"},
			want: "part: b c",
		},
		{
			name: "mixed placeholders",
			body: "cmd=$1 rest=${@:2}",
			args: []string{"run", "tests", "now"},
			want: "cmd=run rest=tests now",
		},
		{
			name: "all nine positional",
			body: "$1$2$3$4$5$6$7$8$9",
			args: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"},
			want: "abcdefghi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expandTemplate(tt.body, tt.args)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePromptFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantDesc string
		wantBody string
	}{
		{
			name:     "no frontmatter",
			input:    "Just a body\nwith lines",
			wantDesc: "",
			wantBody: "Just a body\nwith lines",
		},
		{
			name:     "with frontmatter",
			input:    "---\ndescription: Review code\n---\nReview: $@",
			wantDesc: "Review code",
			wantBody: "Review: $@",
		},
		{
			name:     "empty frontmatter",
			input:    "---\n---\nBody here",
			wantDesc: "",
			wantBody: "Body here",
		},
		{
			name:     "frontmatter without description",
			input:    "---\nother: value\n---\nBody",
			wantDesc: "",
			wantBody: "Body",
		},
		{
			name:     "unclosed frontmatter treated as body",
			input:    "---\nno closing\ndelimiter",
			wantDesc: "",
			wantBody: "---\nno closing\ndelimiter",
		},
		{
			name:     "body with leading whitespace trimmed",
			input:    "---\ndescription: test\n---\n\n  content  \n",
			wantDesc: "test",
			wantBody: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desc, body := parsePromptFile([]byte(tt.input))
			assert.Equal(t, tt.wantDesc, desc)
			assert.Equal(t, tt.wantBody, body)
		})
	}
}
