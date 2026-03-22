package plan_test

import (
	"strings"
	"testing"

	"github.com/dotcommander/piglet/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStore creates a Store whose backing directory lives entirely inside t's
// temp dir so no real config directories are touched.
func newStore(t *testing.T) *plan.Store {
	t.Helper()
	s, err := plan.NewStore(t.TempDir())
	require.NoError(t, err)
	return s
}

// --- Plan creation ----------------------------------------------------------

func TestNewPlan(t *testing.T) {
	t.Parallel()

	t.Run("valid title and steps", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("My Plan", []string{"step one", "step two"})
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, "My Plan", p.Title)
		assert.Equal(t, "my-plan", p.Slug)
		assert.Len(t, p.Steps, 2)
		assert.Equal(t, 1, p.Steps[0].ID)
		assert.Equal(t, "step one", p.Steps[0].Text)
		assert.Equal(t, plan.StatusPending, p.Steps[0].Status)
		assert.Equal(t, 2, p.Steps[1].ID)
		assert.True(t, p.Active)
	})

	t.Run("empty title returns error", func(t *testing.T) {
		t.Parallel()
		_, err := plan.NewPlan("", []string{"step"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "title")
	})

	t.Run("whitespace-only title returns error", func(t *testing.T) {
		t.Parallel()
		_, err := plan.NewPlan("   ", []string{"step"})
		require.Error(t, err)
	})

	t.Run("no steps returns error", func(t *testing.T) {
		t.Parallel()
		_, err := plan.NewPlan("Valid Title", []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "step")
	})
}

func TestSlugGeneration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		title string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  leading spaces  ", "leading-spaces"},
		{"multiple   spaces", "multiple-spaces"},
		{"special!@#chars", "special-chars"},
		{"123 Numbers", "123-numbers"},
		{"already-slugged", "already-slugged"},
		{"Ünïcödé Title", "ünïcödé-title"},
		{"trailing---", "trailing"},
		{"ALLCAPS", "allcaps"},
		{"a", "a"},
	}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			t.Parallel()
			p, err := plan.NewPlan(tc.title, []string{"step"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.Slug)
		})
	}
}

// --- Step operations --------------------------------------------------------

func TestUpdateStep(t *testing.T) {
	t.Parallel()

	t.Run("status transition pending to in_progress", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha", "beta"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusInProgress, ""))
		assert.Equal(t, plan.StatusInProgress, p.Steps[0].Status)
	})

	t.Run("setting in_progress moves previous in_progress to pending", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha", "beta", "gamma"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusInProgress, ""))
		assert.Equal(t, plan.StatusInProgress, p.Steps[0].Status)

		require.NoError(t, p.UpdateStep(2, plan.StatusInProgress, ""))
		assert.Equal(t, plan.StatusPending, p.Steps[0].Status, "step 1 should revert to pending")
		assert.Equal(t, plan.StatusInProgress, p.Steps[1].Status)
	})

	t.Run("done status does not affect other steps", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha", "beta"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusInProgress, ""))
		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		assert.Equal(t, plan.StatusDone, p.Steps[0].Status)
		assert.Equal(t, plan.StatusPending, p.Steps[1].Status)
	})

	t.Run("update with notes", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, "", "my note here"))
		assert.Equal(t, "my note here", p.Steps[0].Notes)
		assert.Equal(t, plan.StatusPending, p.Steps[0].Status, "status unchanged when not specified")
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		err = p.UpdateStep(1, "bogus", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid status")
	})

	t.Run("invalid step ID returns error", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		err = p.UpdateStep(99, plan.StatusDone, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestAddStepAfter(t *testing.T) {
	t.Parallel()

	t.Run("inserts after specified step", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"first", "third"})
		require.NoError(t, err)

		require.NoError(t, p.AddStepAfter(1, "second"))
		require.Len(t, p.Steps, 3)
		assert.Equal(t, "first", p.Steps[0].Text)
		assert.Equal(t, "second", p.Steps[1].Text)
		assert.Equal(t, "third", p.Steps[2].Text)
	})

	t.Run("new step gets next available ID", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"first", "second"})
		require.NoError(t, err)

		require.NoError(t, p.AddStepAfter(1, "inserted"))
		inserted := p.Steps[1]
		assert.Equal(t, 3, inserted.ID, "new step ID should be max+1")
	})

	t.Run("new step has pending status", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		require.NoError(t, p.AddStepAfter(1, "beta"))
		assert.Equal(t, plan.StatusPending, p.Steps[1].Status)
	})

	t.Run("invalid ID returns error", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		err = p.AddStepAfter(99, "new")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRemoveStep(t *testing.T) {
	t.Parallel()

	t.Run("removes correct step", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha", "beta", "gamma"})
		require.NoError(t, err)

		require.NoError(t, p.RemoveStep(2))
		require.Len(t, p.Steps, 2)
		assert.Equal(t, "alpha", p.Steps[0].Text)
		assert.Equal(t, "gamma", p.Steps[1].Text)
	})

	t.Run("does not renumber remaining steps", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha", "beta", "gamma"})
		require.NoError(t, err)

		require.NoError(t, p.RemoveStep(2))
		assert.Equal(t, 1, p.Steps[0].ID, "ID 1 unchanged")
		assert.Equal(t, 3, p.Steps[1].ID, "ID 3 unchanged (not renumbered to 2)")
	})

	t.Run("invalid ID returns error", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"alpha"})
		require.NoError(t, err)

		err = p.RemoveStep(99)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestProgress(t *testing.T) {
	t.Parallel()

	t.Run("all pending", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b", "c"})
		require.NoError(t, err)

		done, total := p.Progress()
		assert.Equal(t, 0, done)
		assert.Equal(t, 3, total)
	})

	t.Run("some done", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b", "c"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		require.NoError(t, p.UpdateStep(3, plan.StatusDone, ""))

		done, total := p.Progress()
		assert.Equal(t, 2, done)
		assert.Equal(t, 3, total)
	})

	t.Run("in_progress not counted as done", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusInProgress, ""))

		done, total := p.Progress()
		assert.Equal(t, 0, done)
		assert.Equal(t, 2, total)
	})

	t.Run("skipped not counted as done", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusSkipped, ""))

		done, total := p.Progress()
		assert.Equal(t, 0, done)
		assert.Equal(t, 2, total)
	})

	t.Run("all done", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)

		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		require.NoError(t, p.UpdateStep(2, plan.StatusDone, ""))

		done, total := p.Progress()
		assert.Equal(t, 2, done)
		assert.Equal(t, 2, total)
	})
}

// --- Store ------------------------------------------------------------------

func TestStoreSaveLoad(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Round Trip", []string{"do this", "do that"})
	require.NoError(t, err)

	require.NoError(t, s.Save(p))

	got, err := s.Load(p.Slug)
	require.NoError(t, err)
	assert.Equal(t, p.Title, got.Title)
	assert.Equal(t, p.Slug, got.Slug)
	assert.Equal(t, p.Active, got.Active)
	require.Len(t, got.Steps, 2)
	assert.Equal(t, p.Steps[0].Text, got.Steps[0].Text)
	assert.Equal(t, p.Steps[1].Text, got.Steps[1].Text)
}

func TestStoreActiveNone(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	got, err := s.Active()
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStoreActive(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Active Plan", []string{"step"})
	require.NoError(t, err)
	require.NoError(t, s.Save(p))

	got, err := s.Active()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.Slug, got.Slug)
}

func TestStoreList(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p1, err := plan.NewPlan("Alpha Plan", []string{"step"})
	require.NoError(t, err)
	p1.Active = false
	require.NoError(t, s.Save(p1))

	p2, err := plan.NewPlan("Beta Plan", []string{"step"})
	require.NoError(t, err)
	p2.Active = false
	require.NoError(t, s.Save(p2))

	plans, err := s.List()
	require.NoError(t, err)
	assert.Len(t, plans, 2)

	slugs := make([]string, len(plans))
	for i, p := range plans {
		slugs[i] = p.Slug
	}
	assert.Contains(t, slugs, p1.Slug)
	assert.Contains(t, slugs, p2.Slug)
}

func TestStoreSetActive(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p1, err := plan.NewPlan("First Plan", []string{"step"})
	require.NoError(t, err)
	require.NoError(t, s.Save(p1))

	p2, err := plan.NewPlan("Second Plan", []string{"step"})
	require.NoError(t, err)
	p2.Active = false
	require.NoError(t, s.Save(p2))

	// Switch active to p2.
	require.NoError(t, s.SetActive(p2.Slug))

	active, err2 := s.Active()
	require.NoError(t, err2)
	require.NotNil(t, active)
	assert.Equal(t, p2.Slug, active.Slug)

	// p1 should no longer be active.
	loaded1, err := s.Load(p1.Slug)
	require.NoError(t, err)
	assert.False(t, loaded1.Active)
}

func TestStoreSetActiveNotFound(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	err := s.SetActive("no-such-plan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStoreDeactivate(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Active Plan", []string{"step"})
	require.NoError(t, err)
	require.NoError(t, s.Save(p))

	activeBefore, err2 := s.Active()
	require.NoError(t, err2)
	require.NotNil(t, activeBefore)

	require.NoError(t, s.Deactivate())

	activeAfter, err3 := s.Active()
	require.NoError(t, err3)
	assert.Nil(t, activeAfter, "should have no active plan after Deactivate")

	// Persisted file should reflect the change.
	loaded, err := s.Load(p.Slug)
	require.NoError(t, err)
	assert.False(t, loaded.Active)
}

func TestStoreDelete(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Doomed Plan", []string{"step"})
	require.NoError(t, err)
	require.NoError(t, s.Save(p))

	require.NoError(t, s.Delete(p.Slug))

	plans, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, plans)

	// Load should fail after delete.
	_, err = s.Load(p.Slug)
	require.Error(t, err)
}

func TestStoreDeleteNonExistent(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	// Delete of a non-existent slug should not return an error (idempotent).
	err := s.Delete("does-not-exist")
	assert.NoError(t, err)
}

// --- Format -----------------------------------------------------------------

func TestFormatPromptNil(t *testing.T) {
	t.Parallel()

	out := plan.FormatPrompt(nil)
	assert.Empty(t, out)
}

func TestFormatPromptStatuses(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("My Task", []string{"pending step", "active step", "done step", "skipped step"})
	require.NoError(t, err)

	require.NoError(t, p.UpdateStep(2, plan.StatusInProgress, ""))
	require.NoError(t, p.UpdateStep(3, plan.StatusDone, ""))
	require.NoError(t, p.UpdateStep(4, plan.StatusSkipped, ""))

	out := plan.FormatPrompt(p)

	assert.Contains(t, out, "## Active Plan: My Task")
	assert.Contains(t, out, "- [ ] 1. pending step")
	assert.Contains(t, out, "**2. active step**")
	assert.Contains(t, out, "← in progress")
	assert.Contains(t, out, "- [x] 3. done step")
	assert.Contains(t, out, "- [-] 4. skipped step")
}

func TestFormatPromptNotes(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Noted Plan", []string{"step with note"})
	require.NoError(t, err)

	require.NoError(t, p.UpdateStep(1, "", "this is a note"))

	out := plan.FormatPrompt(p)
	assert.Contains(t, out, "  - this is a note")
}

func TestFormatPromptProgress(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Progress Plan", []string{"a", "b", "c"})
	require.NoError(t, err)

	require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))

	out := plan.FormatPrompt(p)
	assert.True(t, strings.Contains(out, "1/3"), "expected progress footer with 1/3 done, got: %s", out)
}

func TestFormatPromptProgressZero(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Zero Progress", []string{"step one", "step two"})
	require.NoError(t, err)

	out := plan.FormatPrompt(p)
	assert.Contains(t, out, "0/2")
}

// --- IsComplete -------------------------------------------------------------

func TestIsComplete(t *testing.T) {
	t.Parallel()

	t.Run("all done returns true", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)
		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		require.NoError(t, p.UpdateStep(2, plan.StatusDone, ""))
		assert.True(t, p.IsComplete())
	})

	t.Run("mix of done skipped failed returns true", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b", "c"})
		require.NoError(t, err)
		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		require.NoError(t, p.UpdateStep(2, plan.StatusSkipped, ""))
		require.NoError(t, p.UpdateStep(3, plan.StatusFailed, ""))
		assert.True(t, p.IsComplete())
	})

	t.Run("has pending returns false", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)
		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		// step 2 remains pending
		assert.False(t, p.IsComplete())
	})

	t.Run("has in_progress returns false", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b"})
		require.NoError(t, err)
		require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
		require.NoError(t, p.UpdateStep(2, plan.StatusInProgress, ""))
		assert.False(t, p.IsComplete())
	})

	t.Run("empty steps returns false", func(t *testing.T) {
		t.Parallel()
		// Cannot use NewPlan with zero steps, so construct directly.
		p := &plan.Plan{}
		assert.False(t, p.IsComplete())
	})
}

// --- Failed status ----------------------------------------------------------

func TestUpdateStepFailed(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Fail Test", []string{"step one"})
	require.NoError(t, err)
	require.NoError(t, p.UpdateStep(1, plan.StatusFailed, ""))
	assert.Equal(t, plan.StatusFailed, p.Steps[0].Status)

	require.NoError(t, s.Save(p))

	loaded, err := s.Load(p.Slug)
	require.NoError(t, err)
	assert.Equal(t, plan.StatusFailed, loaded.Steps[0].Status)
}

func TestFormatPromptFailed(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Fail Format", []string{"good step", "bad step"})
	require.NoError(t, err)

	require.NoError(t, p.UpdateStep(1, plan.StatusDone, ""))
	require.NoError(t, p.UpdateStep(2, plan.StatusFailed, ""))

	out := plan.FormatPrompt(p)
	assert.Contains(t, out, "- [x] 1. good step")
	assert.Contains(t, out, "- [!] 2. bad step")
}

// --- Mode -------------------------------------------------------------------

func TestInProposeMode(t *testing.T) {
	t.Parallel()

	t.Run("propose mode returns true", func(t *testing.T) {
		t.Parallel()
		p := &plan.Plan{Mode: plan.ModePropose}
		assert.True(t, p.InProposeMode())
	})

	t.Run("execute mode returns false", func(t *testing.T) {
		t.Parallel()
		p := &plan.Plan{Mode: plan.ModeExecute}
		assert.False(t, p.InProposeMode())
	})

	t.Run("empty mode returns false", func(t *testing.T) {
		t.Parallel()
		p := &plan.Plan{}
		assert.False(t, p.InProposeMode())
	})
}

func TestAppendStep(t *testing.T) {
	t.Parallel()

	t.Run("appends to end", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"first", "second"})
		require.NoError(t, err)

		p.AppendStep("third")
		require.Len(t, p.Steps, 3)
		assert.Equal(t, "third", p.Steps[2].Text)
		assert.Equal(t, 3, p.Steps[2].ID)
		assert.Equal(t, plan.StatusPending, p.Steps[2].Status)
	})

	t.Run("ID is max+1 after removal", func(t *testing.T) {
		t.Parallel()
		p, err := plan.NewPlan("Test", []string{"a", "b", "c"})
		require.NoError(t, err)

		require.NoError(t, p.RemoveStep(2))
		p.AppendStep("d")
		assert.Equal(t, 4, p.Steps[2].ID)
	})
}

func TestModeRoundTrip(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	p, err := plan.NewPlan("Mode Test", []string{"step"})
	require.NoError(t, err)
	p.Mode = plan.ModePropose

	require.NoError(t, s.Save(p))

	loaded, err := s.Load(p.Slug)
	require.NoError(t, err)
	assert.Equal(t, plan.ModePropose, loaded.Mode)
	assert.True(t, loaded.InProposeMode())
}

func TestFormatPromptProposeMode(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Propose Plan", []string{"step one"})
	require.NoError(t, err)
	p.Mode = plan.ModePropose

	out := plan.FormatPrompt(p)
	assert.Contains(t, out, "MODE: PROPOSE")
	assert.Contains(t, out, "Mutating tools")
}

func TestFormatPromptExecuteMode(t *testing.T) {
	t.Parallel()

	p, err := plan.NewPlan("Execute Plan", []string{"step one"})
	require.NoError(t, err)
	p.Mode = plan.ModeExecute

	out := plan.FormatPrompt(p)
	assert.NotContains(t, out, "MODE: PROPOSE")
}
