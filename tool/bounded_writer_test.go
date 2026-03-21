package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoundedWriter_UnderLimit(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 100}
	n, err := w.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", w.String())
	assert.Equal(t, 5, w.Len())
	assert.False(t, w.Truncated())
}

func TestBoundedWriter_ExactLimit(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 5}
	n, err := w.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", w.String())
	assert.True(t, w.Truncated())
}

func TestBoundedWriter_SingleWriteOverLimit(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 3}
	n, err := w.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n, "must return len(p) to satisfy io.Writer contract")
	assert.Equal(t, "hel", w.String())
	assert.True(t, w.Truncated())
}

func TestBoundedWriter_MultipleWrites(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 8}
	w.Write([]byte("hello"))
	w.Write([]byte(" world"))
	assert.Equal(t, "hello wo", w.String())
	assert.True(t, w.Truncated())
}

func TestBoundedWriter_WritesAfterFull(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 3}
	w.Write([]byte("abc"))
	n, err := w.Write([]byte("def"))
	assert.NoError(t, err)
	assert.Equal(t, 3, n, "must return len(p) even when discarding")
	assert.Equal(t, "abc", w.String())
}

func TestBoundedWriter_EmptyWrite(t *testing.T) {
	t.Parallel()
	w := &boundedWriter{limit: 10}
	n, err := w.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, "", w.String())
	assert.False(t, w.Truncated())
}
