package sessioncmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatShortTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid RFC3339",
			input: "2024-03-15T14:22:37Z",
			want:  "14:22:37",
		},
		{
			name: "valid RFC3339 with offset",
			// time.Parse preserves timezone; Format prints in parsed zone (not UTC).
			input: "2024-03-15T14:22:37+05:30",
			want:  "14:22:37",
		},
		{
			name:  "invalid returns original",
			input: "not-a-timestamp",
			want:  "not-a-timestamp",
		},
		{
			name:  "empty returns empty",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, formatShortTimestamp(tc.input))
		})
	}
}
