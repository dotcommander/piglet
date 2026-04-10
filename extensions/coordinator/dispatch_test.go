package coordinator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeResults(t *testing.T) {
	t.Parallel()

	t.Run("single result", func(t *testing.T) {
		t.Parallel()
		results := []DispatchResult{
			{Index: 0, Task: "test", Text: "hello world", Turns: 3, InputTok: 1000, OutputTok: 500},
		}
		merged := MergeResults(results)
		assert.Contains(t, merged, "coordinator:")
		assert.Contains(t, merged, "hello world")
		assert.NotContains(t, merged, "Task 1", "single task should not have task header")
	})

	t.Run("multiple results", func(t *testing.T) {
		t.Parallel()
		results := []DispatchResult{
			{Index: 0, Task: "task1", Text: "result one", Turns: 2, InputTok: 1000, OutputTok: 500},
			{Index: 1, Task: "task2", Text: "result two", Turns: 3, InputTok: 2000, OutputTok: 800},
		}
		merged := MergeResults(results)
		assert.Contains(t, merged, "2 task(s)")
		assert.Contains(t, merged, "Task 1")
		assert.Contains(t, merged, "Task 2")
		assert.Contains(t, merged, "result one")
		assert.Contains(t, merged, "result two")
	})

	t.Run("error result", func(t *testing.T) {
		t.Parallel()
		results := []DispatchResult{
			{Index: 0, Task: "bad task", Error: "agent failed"},
		}
		merged := MergeResults(results)
		assert.Contains(t, merged, "Error: agent failed")
	})
}

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	tasks := []SubTask{
		{Task: "do something", Tools: "", Model: "", MaxTurns: 0},
		{Task: "another", Tools: "all", Model: "small", MaxTurns: maxSubTaskTurns + 5},
	}

	result := applyDefaults(tasks)

	assert.Equal(t, "all", result[0].Tools)
	assert.Equal(t, "default", result[0].Model)
	assert.Equal(t, maxSubTaskTurns, result[0].MaxTurns)
	assert.Equal(t, maxSubTaskTurns, result[1].MaxTurns, "should cap at maxSubTaskTurns when exceeded")
}
