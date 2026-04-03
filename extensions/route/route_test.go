package route

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "basic sentence",
			input:  "fix the goroutine leak in handler",
			expect: []string{"fix", "goroutine", "leak", "handler"},
		},
		{
			name:   "preserves hyphens",
			input:  "debug the race-condition",
			expect: []string{"debug", "race-condition"},
		},
		{
			name:   "removes duplicates",
			input:  "test test test debug",
			expect: []string{"test", "debug"},
		},
		{
			name:   "removes single chars",
			input:  "a b c debug",
			expect: []string{"debug"},
		},
		{
			name:   "empty input",
			input:  "",
			expect: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Tokenize(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestNormalizePlural(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "goroutine", NormalizePlural("goroutines"))
	assert.Equal(t, "test", NormalizePlural("tests"))
	assert.Equal(t, "ss", NormalizePlural("ss"))       // too short
	assert.Equal(t, "go", NormalizePlural("go"))       // no trailing s
	assert.Equal(t, "class", NormalizePlural("class")) // ends in ss
}

func TestTokensContain(t *testing.T) {
	t.Parallel()

	tokens := []string{"goroutine", "leak", "handler"}
	assert.True(t, TokensContain(tokens, "goroutine"))
	assert.True(t, TokensContain(tokens, "goroutines")) // plural normalization
	assert.True(t, TokensContain(tokens, "leak"))
	assert.False(t, TokensContain(tokens, "debug"))
}

func TestTokensContainAll(t *testing.T) {
	t.Parallel()

	tokens := []string{"goroutine", "leak", "handler"}
	assert.True(t, TokensContainAll(tokens, "goroutine leak"))
	assert.False(t, TokensContainAll(tokens, "goroutine race"))
}

func testIntentClassifier() *IntentClassifier {
	return NewIntentClassifier(IntentsConfig{
		Intents: map[string]IntentDef{
			"debug": {
				Verbs:    []string{"fix", "debug", "diagnose"},
				Keywords: []string{"bug", "error", "crash", "leak", "race"},
			},
			"test": {
				Verbs:    []string{"test", "verify", "validate"},
				Keywords: []string{"test", "coverage", "assertion"},
			},
			"refactor": {
				Verbs:    []string{"refactor", "restructure", "simplify"},
				Keywords: []string{"refactor", "cleanup", "dead code"},
			},
			"search": {
				Verbs:    []string{"find", "search", "grep"},
				Keywords: []string{"find", "search", "where", "locate"},
			},
			"write": {
				Verbs:    []string{"write", "add", "implement", "create"},
				Keywords: []string{"feature", "implement", "new", "add"},
			},
		},
	})
}

func TestIntentClassify_LeadingVerb(t *testing.T) {
	t.Parallel()

	ic := testIntentClassifier()
	result := ic.Classify("fix the goroutine leak in handler")

	assert.Equal(t, "debug", result.Primary)
	assert.Equal(t, 0.9, result.Confidence)
}

func TestIntentClassify_Question(t *testing.T) {
	t.Parallel()

	ic := testIntentClassifier()
	result := ic.Classify("how do I fix the memory leak?")

	assert.Equal(t, "debug", result.Primary)
	assert.Equal(t, 0.85, result.Confidence)
}

func TestIntentClassify_ProblemKeyword(t *testing.T) {
	t.Parallel()

	ic := testIntentClassifier()
	result := ic.Classify("the handler has a race condition")

	assert.Equal(t, "debug", result.Primary)
	assert.Equal(t, 0.7, result.Confidence)
}

func TestIntentClassify_CompoundIntent(t *testing.T) {
	t.Parallel()

	ic := testIntentClassifier()
	result := ic.Classify("refactor the handler and test it")

	assert.Equal(t, "refactor", result.Primary)
	assert.Equal(t, "test", result.Secondary)
}

func TestIntentClassify_Empty(t *testing.T) {
	t.Parallel()

	ic := testIntentClassifier()
	result := ic.Classify("")

	assert.Empty(t, result.Primary)
}

func testDomainExtractor() *DomainExtractor {
	return NewDomainExtractor(DomainsConfig{
		Domains: map[string]DomainDef{
			"go": {
				Keywords:   []string{"goroutine", "gomod"},
				Projects:   []string{"go.mod"},
				Extensions: []string{".go"},
			},
			"typescript": {
				Keywords:   []string{"typescript", "tsx"},
				Projects:   []string{"tsconfig.json"},
				Extensions: []string{".ts", ".tsx"},
			},
			"concurrency": {
				Keywords:   []string{"goroutine", "mutex", "deadlock", "race", "channel"},
				Projects:   nil,
				Extensions: nil,
			},
		},
	})
}

func TestDomainExtract_Keywords(t *testing.T) {
	t.Parallel()

	de := testDomainExtractor()
	domains := de.Extract("fix the goroutine leak", "")

	assert.Contains(t, domains, "go")
	assert.Contains(t, domains, "concurrency")
}

func TestDomainExtract_FileRef(t *testing.T) {
	t.Parallel()

	de := testDomainExtractor()
	domains := de.Extract("check `handler.go` for issues", "")

	assert.Contains(t, domains, "go")
}

func TestDomainExtract_ProjectDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))

	de := testDomainExtractor()
	domains := de.Extract("check for issues", dir)

	assert.Contains(t, domains, "go")
}

func TestScorer_FullPipeline(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	ic := testIntentClassifier()
	de := testDomainExtractor()
	scorer := NewScorer(cfg, ic, de)

	reg := &Registry{
		Components: []Component{
			{
				Name:      "confirm",
				Type:      TypeExtension,
				Extension: "confirm",
				Keywords:  []string{"confirm", "verify", "test", "lint"},
				Intents:   []string{"test", "review"},
			},
			{
				Name:        "dispatch",
				Type:        TypeTool,
				Extension:   "subagent",
				Description: "Delegate a task to an independent sub-agent",
				Keywords:    []string{"dispatch", "delegate", "task", "sub-agent"},
				Intents:     []string{"write", "debug", "search"},
			},
			{
				Name:      "plan_create",
				Type:      TypeTool,
				Extension: "plan",
				Keywords:  []string{"plan", "create", "steps"},
				Intents:   []string{"design", "write"},
			},
			{
				Name:        "fossil",
				Type:        TypeExtension,
				Extension:   "fossil",
				Description: "Git history queries",
				Keywords:    []string{"fossil", "git", "blame", "changes", "history"},
				Intents:     []string{"debug", "search"},
				Domains:     []string{"git"},
			},
		},
	}

	projDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "go.mod"), []byte("module test\n"), 0o644))

	result := scorer.Score("fix the goroutine leak in handler.go", projDir, reg)

	require.Equal(t, "debug", result.Intent.Primary)
	require.Contains(t, result.Domains, "go")

	// Should have some scored results
	total := len(result.Primary) + len(result.Secondary)
	assert.Greater(t, total, 0, "expected at least one scored component")
}

func TestScorer_DeclaredIntentsScoreHigher(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	ic := testIntentClassifier()
	de := testDomainExtractor()
	scorer := NewScorer(cfg, ic, de)

	reg := &Registry{
		Components: []Component{
			{
				Name:      "with-intents",
				Type:      TypeExtension,
				Extension: "with-intents",
				Intents:   []string{"debug"},
				Keywords:  []string{"generic"},
			},
			{
				Name:        "without-intents",
				Type:        TypeExtension,
				Extension:   "without-intents",
				Description: "handles debug stuff",
				Keywords:    []string{"generic"},
			},
		},
	}

	result := scorer.Score("debug the crash", "", reg)

	// Both should score, but declared intents should score higher
	var withScore, withoutScore float64
	for _, sc := range append(result.Primary, result.Secondary...) {
		switch sc.Name {
		case "with-intents":
			withScore = sc.Score
		case "without-intents":
			withoutScore = sc.Score
		}
	}
	assert.Greater(t, withScore, withoutScore, "declared intents should score higher than heuristic")
}

func TestScorer_DeclaredDomainsMatch(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	ic := testIntentClassifier()
	de := testDomainExtractor()
	scorer := NewScorer(cfg, ic, de)

	reg := &Registry{
		Components: []Component{
			{
				Name:      "git-tool",
				Type:      TypeExtension,
				Extension: "git-tool",
				Domains:   []string{"git"},
				Keywords:  []string{"tool"},
			},
			{
				Name:      "unrelated",
				Type:      TypeExtension,
				Extension: "unrelated",
				Domains:   []string{"frontend"},
				Keywords:  []string{"tool"},
			},
		},
	}

	// "goroutine" triggers go+concurrency domains, not git or frontend
	result := scorer.Score("fix the goroutine leak", "", reg)

	// git-tool and unrelated both have non-matching declared domains, so both get 0 domain score
	for _, sc := range append(result.Primary, result.Secondary...) {
		if sc.Name == "unrelated" {
			assert.Less(t, sc.Score, 0.3, "unrelated domain should score low")
		}
	}
}

func TestScorer_AntiTriggersPenalize(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	ic := testIntentClassifier()
	de := testDomainExtractor()
	scorer := NewScorer(cfg, ic, de)

	reg := &Registry{
		Components: []Component{
			{
				Name:      "no-anti",
				Type:      TypeExtension,
				Extension: "no-anti",
				Intents:   []string{"debug"},
				Keywords:  []string{"goroutine", "leak"},
			},
			{
				Name:         "with-anti",
				Type:         TypeExtension,
				Extension:    "with-anti",
				Intents:      []string{"debug"},
				Keywords:     []string{"goroutine", "leak"},
				AntiTriggers: []string{"goroutine", "leak"},
			},
		},
	}

	result := scorer.Score("fix the goroutine leak", "", reg)

	var noAntiScore, withAntiScore float64
	for _, sc := range append(result.Primary, result.Secondary...) {
		switch sc.Name {
		case "no-anti":
			noAntiScore = sc.Score
		case "with-anti":
			withAntiScore = sc.Score
		}
	}
	assert.Greater(t, noAntiScore, 0.0, "no-anti should score positive")
	assert.Greater(t, noAntiScore, withAntiScore, "anti-triggers should penalize score")
}

func TestScorer_AntiTriggersNoMatchNoPenalty(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	ic := testIntentClassifier()
	de := testDomainExtractor()
	scorer := NewScorer(cfg, ic, de)

	reg := &Registry{
		Components: []Component{
			{
				Name:      "baseline",
				Type:      TypeExtension,
				Extension: "baseline",
				Intents:   []string{"debug"},
				Keywords:  []string{"goroutine"},
			},
			{
				Name:         "unrelated-anti",
				Type:         TypeExtension,
				Extension:    "unrelated-anti",
				Intents:      []string{"debug"},
				Keywords:     []string{"goroutine"},
				AntiTriggers: []string{"typescript", "frontend"},
			},
		},
	}

	result := scorer.Score("fix the goroutine leak", "", reg)

	var baseScore, antiScore float64
	for _, sc := range append(result.Primary, result.Secondary...) {
		switch sc.Name {
		case "baseline":
			baseScore = sc.Score
		case "unrelated-anti":
			antiScore = sc.Score
		}
	}
	assert.Equal(t, baseScore, antiScore, "non-matching anti-triggers should not penalize")
}

func TestMergeLearnedAntiTriggers(t *testing.T) {
	t.Parallel()

	reg := &Registry{
		Components: []Component{
			{
				Name:      "plan_create",
				Type:      TypeTool,
				Extension: "plan",
			},
		},
	}

	lt := &LearnedTriggers{
		Triggers: make(map[string][]string),
		AntiTriggers: map[string][]string{
			"plan_create": {"goroutine", "leak"},
		},
	}

	mergeLearnedIntoRegistry(reg, lt)

	assert.Contains(t, reg.Components[0].AntiTriggers, "goroutine")
	assert.Contains(t, reg.Components[0].AntiTriggers, "leak")
}

func TestFormatHookContext(t *testing.T) {
	t.Parallel()

	r := RouteResult{
		Intent:  IntentResult{Primary: "debug"},
		Domains: []string{"go", "concurrency"},
		Primary: []ScoredComponent{
			{Name: "confirm", Type: TypeExtension, Score: 0.5},
			{Name: "dispatch", Type: TypeTool, Score: 0.3},
		},
		Confidence: 0.2,
	}

	got := FormatHookContext(r)
	assert.Contains(t, got, "intent=debug")
	assert.Contains(t, got, "domains=go,concurrency")
	assert.Contains(t, got, "confirm")
	assert.Contains(t, got, "dispatch")
}

func TestFormatHookContext_Empty(t *testing.T) {
	t.Parallel()

	r := RouteResult{}
	got := FormatHookContext(r)
	assert.Empty(t, got)
}

func TestFeedbackStore_RecordAndLearn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs := NewFeedbackStore(dir)

	// Record some feedback
	require.NoError(t, fs.Record("fix the goroutine leak", "dispatch", true))
	require.NoError(t, fs.Record("fix the goroutine leak", "plan_create", false))
	require.NoError(t, fs.Record("search for the handler", "repomap", true))

	// Learn from feedback
	lt, err := fs.Learn()
	require.NoError(t, err)

	// Correct feedback should create triggers
	assert.NotEmpty(t, lt.Triggers["dispatch"], "dispatch should have learned triggers")
	assert.Contains(t, lt.Triggers["dispatch"], "goroutine")

	// Wrong feedback should create anti-triggers
	assert.NotEmpty(t, lt.AntiTriggers["plan_create"], "plan_create should have anti-triggers")

	// Correct search feedback
	assert.NotEmpty(t, lt.Triggers["repomap"])
}

func TestFeedbackStore_LoadLearned_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs := NewFeedbackStore(dir)

	lt := fs.LoadLearned()
	assert.NotNil(t, lt.Triggers)
	assert.NotNil(t, lt.AntiTriggers)
	assert.Empty(t, lt.Triggers)
	assert.Empty(t, lt.AntiTriggers)
}

func TestLogRoute(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	result := RouteResult{
		Intent:     IntentResult{Primary: "debug", Confidence: 0.9},
		Domains:    []string{"go"},
		Primary:    []ScoredComponent{{Name: "dispatch", Score: 0.5}},
		Confidence: 0.5,
	}

	logRoute(dir, result, "abc123", "tool")

	data, err := os.ReadFile(filepath.Join(dir, "route-log.jsonl"))
	require.NoError(t, err)

	assert.Contains(t, string(data), `"intent":"debug"`)
	assert.Contains(t, string(data), `"source":"tool"`)
	assert.Contains(t, string(data), `"prompt_hash":"abc123"`)
	assert.Contains(t, string(data), `dispatch:0.50`)
}

func TestLogRoute_EmptyDir(t *testing.T) {
	t.Parallel()

	// Should not panic with empty dir
	logRoute("", RouteResult{}, "abc", "tool")
}

func TestMergeLearnedIntoRegistry(t *testing.T) {
	t.Parallel()

	reg := &Registry{
		Components: []Component{
			{
				Name:      "dispatch",
				Type:      TypeTool,
				Extension: "subagent",
				Keywords:  []string{"dispatch", "delegate"},
			},
		},
	}

	lt := &LearnedTriggers{
		Triggers: map[string][]string{
			"dispatch": {"goroutine", "leak"},
		},
		AntiTriggers: make(map[string][]string),
	}

	mergeLearnedIntoRegistry(reg, lt)

	assert.Contains(t, reg.Components[0].Keywords, "goroutine")
	assert.Contains(t, reg.Components[0].Keywords, "leak")
	assert.Contains(t, reg.Components[0].Keywords, "dispatch") // original preserved
}
