package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExactScorer(t *testing.T) {
	t.Parallel()

	s := &ExactScorer{}
	ctx := context.Background()

	t.Run("match", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "Hello to you", "Hello to you", "")
		require.NoError(t, err)
		assert.Equal(t, 1.0, result.Score)
		assert.True(t, result.Pass)
	})

	t.Run("match with surrounding whitespace", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "  Hello to you  ", "Hello to you", "")
		require.NoError(t, err)
		assert.Equal(t, 1.0, result.Score)
		assert.True(t, result.Pass)
	})

	t.Run("mismatch", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "Hi there", "Hello to you", "")
		require.NoError(t, err)
		assert.Equal(t, 0.0, result.Score)
		assert.False(t, result.Pass)
	})
}

func TestContainsScorer(t *testing.T) {
	t.Parallel()

	s := &ContainsScorer{}
	ctx := context.Background()

	t.Run("substring present", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "Here is a func example", "func", "")
		require.NoError(t, err)
		assert.Equal(t, 1.0, result.Score)
		assert.True(t, result.Pass)
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "Here is a FUNC example", "func", "")
		require.NoError(t, err)
		assert.Equal(t, 1.0, result.Score)
		assert.True(t, result.Pass)
	})

	t.Run("substring absent", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "This is just text", "func", "")
		require.NoError(t, err)
		assert.Equal(t, 0.0, result.Score)
		assert.False(t, result.Pass)
	})
}

func TestRegexScorer(t *testing.T) {
	t.Parallel()

	s := &RegexScorer{}
	ctx := context.Background()

	t.Run("valid match", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "user@example.com", `.+@.+\..+`, "")
		require.NoError(t, err)
		assert.Equal(t, 1.0, result.Score)
		assert.True(t, result.Pass)
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		result, err := s.Score(ctx, "not an email", `.+@.+\..+`, "")
		require.NoError(t, err)
		assert.Equal(t, 0.0, result.Score)
		assert.False(t, result.Pass)
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		t.Parallel()
		_, err := s.Score(ctx, "response", `[invalid(regex`, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compile regex")
	})
}

func TestNewScorerUnknown(t *testing.T) {
	t.Parallel()

	_, err := NewScorer("nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scorer")
}
