package cliutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAllLimit_AllowsInputAtLimit(t *testing.T) {
	t.Parallel()

	got, err := ReadAllLimit(strings.NewReader("abcd"), 4)
	require.NoError(t, err)
	assert.Equal(t, []byte("abcd"), got)
}

func TestReadAllLimit_RejectsInputPastLimit(t *testing.T) {
	t.Parallel()

	got, err := ReadAllLimit(strings.NewReader("abcde"), 4)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInputTooLarge))
	assert.Nil(t, got)
}
