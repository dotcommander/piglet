package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSuite(t *testing.T) {
	t.Parallel()

	yaml := `
name: test-suite
description: A test suite
model: small
cases:
  - name: greeting
    prompt: Say hello
    scorer: exact
    expected: Hello
  - name: check-func
    prompt: Write a Go function
    scorer: contains
    expected: func
  - name: email
    prompt: Give me an email
    scorer: regex
    expected: .+@.+\..+
  - name: quality
    prompt: Explain recursion
    scorer: judge
    criteria: Clear explanation with base case mentioned
`

	tmp := filepath.Join(t.TempDir(), "suite.yaml")
	require.NoError(t, os.WriteFile(tmp, []byte(yaml), 0o600))

	s, err := LoadSuite(tmp)
	require.NoError(t, err)

	assert.Equal(t, "test-suite", s.Name)
	assert.Equal(t, "A test suite", s.Description)
	assert.Equal(t, "small", s.Model)
	require.Len(t, s.Cases, 4)

	assert.Equal(t, "greeting", s.Cases[0].Name)
	assert.Equal(t, "Say hello", s.Cases[0].Prompt)
	assert.Equal(t, "exact", s.Cases[0].Scorer)
	assert.Equal(t, "Hello", s.Cases[0].Expected)

	assert.Equal(t, "quality", s.Cases[3].Name)
	assert.Equal(t, "judge", s.Cases[3].Scorer)
	assert.Equal(t, "Clear explanation with base case mentioned", s.Cases[3].Criteria)
}

func TestLoadSuiteValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing suite name",
			yaml: `
description: no name here
cases:
  - name: c1
    prompt: hi
    scorer: exact
`,
			wantErr: "name is required",
		},
		{
			name: "missing case prompt",
			yaml: `
name: suite
cases:
  - name: c1
    scorer: exact
`,
			wantErr: "prompt is required",
		},
		{
			name: "invalid scorer type",
			yaml: `
name: suite
cases:
  - name: c1
    prompt: hi
    scorer: llm-as-judge
`,
			wantErr: "unknown scorer",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmp := filepath.Join(t.TempDir(), "suite.yaml")
			require.NoError(t, os.WriteFile(tmp, []byte(tc.yaml), 0o600))
			_, err := LoadSuite(tmp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestListSuites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	suite1 := `
name: suite-alpha
description: First suite
model: default
cases:
  - name: c1
    prompt: hello
    scorer: exact
    expected: hi
`
	suite2 := `
name: suite-beta
description: Second suite
cases:
  - name: c1
    prompt: world
    scorer: contains
    expected: world
  - name: c2
    prompt: foo
    scorer: regex
    expected: fo+
`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.yaml"), []byte(suite1), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "beta.yaml"), []byte(suite2), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a suite"), 0o600))

	summaries, err := ListSuites(dir)
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	names := make(map[string]SuiteSummary)
	for _, s := range summaries {
		names[s.Name] = s
	}

	a := names["suite-alpha"]
	assert.Equal(t, "First suite", a.Description)
	assert.Equal(t, 1, a.CaseCount)

	b := names["suite-beta"]
	assert.Equal(t, "Second suite", b.Description)
	assert.Equal(t, 2, b.CaseCount)
}
