package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Validate ─────────────────────────────────────────────────────────────────

func TestValidate(t *testing.T) {
	t.Parallel()

	validStep := Step{Name: "s1", Run: "echo ok"}

	tests := []struct {
		name    string
		pipe    Pipeline
		params  map[string]string
		wantErr string
	}{
		{
			name:    "missing name",
			pipe:    Pipeline{Steps: []Step{validStep}},
			params:  nil,
			wantErr: "pipeline name is required",
		},
		{
			name:    "no steps",
			pipe:    Pipeline{Name: "p"},
			params:  nil,
			wantErr: `pipeline "p" has no steps`,
		},
		{
			name: "step missing name",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{{Run: "echo hi"}},
			},
			params:  nil,
			wantErr: "step 0 has no name",
		},
		{
			name: "step missing run",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{{Name: "s1"}},
			},
			params:  nil,
			wantErr: `step "s1" has no run command`,
		},
		{
			name: "required param missing",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true},
				},
			},
			params:  map[string]string{},
			wantErr: `required parameter "env" not provided`,
		},
		{
			name: "required param has default — ok",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true, Default: "dev"},
				},
			},
			params:  map[string]string{},
			wantErr: "",
		},
		{
			name: "required param supplied — ok",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
				Params: map[string]Param{
					"env": {Required: true},
				},
			},
			params:  map[string]string{"env": "prod"},
			wantErr: "",
		},
		{
			name: "happy path",
			pipe: Pipeline{
				Name:  "p",
				Steps: []Step{validStep},
			},
			params:  nil,
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.pipe.Validate(tc.params)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ── MergeParams ───────────────────────────────────────────────────────────────

func TestMergeParams(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		Params: map[string]Param{
			"host": {Default: "localhost"},
			"port": {Default: "5432"},
			"db":   {},
		},
	}

	t.Run("defaults only", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(nil)
		assert.Equal(t, "localhost", got["host"])
		assert.Equal(t, "5432", got["port"])
		assert.NotContains(t, got, "db") // empty default not included
	})

	t.Run("overrides only", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(map[string]string{"host": "prod-db", "extra": "yes"})
		assert.Equal(t, "prod-db", got["host"])
		assert.Equal(t, "5432", got["port"])
		assert.Equal(t, "yes", got["extra"]) // extra key from override
	})

	t.Run("override wins over default", func(t *testing.T) {
		t.Parallel()
		got := p.MergeParams(map[string]string{"port": "9999"})
		assert.Equal(t, "localhost", got["host"])
		assert.Equal(t, "9999", got["port"])
	})
}

// ── Run validation error ──────────────────────────────────────────────────────

func TestRunValidationError(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		// missing name — Validate will fail
		Steps: []Step{{Name: "s", Run: "echo ok"}},
	}

	_, err := Run(context.Background(), p, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline name is required")
}
