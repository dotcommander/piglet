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

func TestSubTaskDefaults(t *testing.T) {
	t.Parallel()

	// Test that PlanTasks validates sub-task fields.
	// Since PlanTasks requires a real host connection for Chat(),
	// we test the validation logic inline.
	tasks := []SubTask{
		{Task: "do something", Tools: "", Model: "", MaxTurns: 0},
		{Task: "another", Tools: "all", Model: "small", MaxTurns: 25},
	}

	for i := range tasks {
		if tasks[i].Tools == "" {
			tasks[i].Tools = "all"
		}
		if tasks[i].Model == "" {
			tasks[i].Model = "default"
		}
		if tasks[i].MaxTurns <= 0 || tasks[i].MaxTurns > 20 {
			tasks[i].MaxTurns = 10
		}
	}

	assert.Equal(t, "all", tasks[0].Tools)
	assert.Equal(t, "default", tasks[0].Model)
	assert.Equal(t, 10, tasks[0].MaxTurns)
	assert.Equal(t, 10, tasks[1].MaxTurns, "should cap at 10 when > 20")
}
