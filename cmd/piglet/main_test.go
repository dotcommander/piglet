package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/dotcommander/piglet/cmd/internal/cliutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadStdinFrom_CombinesPromptArgsAndPipe(t *testing.T) {
	t.Parallel()

	got, err := readStdinFrom([]string{"summarize"}, strings.NewReader("hello\n"))
	require.NoError(t, err)
	assert.Equal(t, "summarize\n\nhello", got)
}

func TestReadStdinFrom_RejectsInputPastLimit(t *testing.T) {
	t.Parallel()

	input := strings.NewReader(strings.Repeat("x", int(cliutil.DefaultMaxStdinBytes)+1))
	got, err := readStdinFrom([]string{"prompt"}, input)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, cliutil.ErrInputTooLarge))
	assert.Equal(t, "prompt", got)
}
