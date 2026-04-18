package main

import (
	"encoding/json"
	"testing"

	"github.com/dotcommander/piglet/extensions/lsp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRefsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		locs     []lsp.Location
		cwd      string
		wantLen  int
		wantFile string
		wantLine int
		wantCol  int
	}{
		{
			name:    "empty locations",
			locs:    nil,
			cwd:     "/project",
			wantLen: 0,
		},
		{
			name: "single reference",
			locs: []lsp.Location{
				{
					URI: "file:///project/main.go",
					Range: lsp.Range{
						Start: lsp.Position{Line: 9, Character: 4},
					},
				},
			},
			cwd:      "/project",
			wantLen:  1,
			wantFile: "main.go",
			wantLine: 10,
			wantCol:  5,
		},
		{
			name: "multiple references",
			locs: []lsp.Location{
				{URI: "file:///project/a.go", Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}},
				{URI: "file:///project/b.go", Range: lsp.Range{Start: lsp.Position{Line: 4, Character: 2}}},
			},
			cwd:     "/project",
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := buildRefsJSON(tt.locs, tt.cwd)
			require.Len(t, out.References, tt.wantLen)
			if tt.wantLen > 0 && tt.wantFile != "" {
				assert.Equal(t, tt.wantFile, out.References[0].File)
				assert.Equal(t, tt.wantLine, out.References[0].Line)
				assert.Equal(t, tt.wantCol, out.References[0].Column)
			}
		})
	}
}

func TestBuildDefJSON(t *testing.T) {
	t.Parallel()

	t.Run("no definition", func(t *testing.T) {
		t.Parallel()
		out := buildDefJSON(nil, "/project")
		assert.Nil(t, out.Definition)

		// Verify JSON serialises correctly.
		data, err := json.Marshal(out)
		require.NoError(t, err)
		assert.JSONEq(t, `{"definition":null}`, string(data))
	})

	t.Run("single definition", func(t *testing.T) {
		t.Parallel()
		locs := []lsp.Location{
			{URI: "file:///project/pkg/server.go", Range: lsp.Range{Start: lsp.Position{Line: 19, Character: 5}}},
		}
		out := buildDefJSON(locs, "/project")
		require.NotNil(t, out.Definition)
		assert.Equal(t, "pkg/server.go", out.Definition.File)
		assert.Equal(t, 20, out.Definition.Line)
		assert.Equal(t, 6, out.Definition.Column)
	})

	t.Run("multiple locs uses first", func(t *testing.T) {
		t.Parallel()
		locs := []lsp.Location{
			{URI: "file:///project/a.go", Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}},
			{URI: "file:///project/b.go", Range: lsp.Range{Start: lsp.Position{Line: 5, Character: 2}}},
		}
		out := buildDefJSON(locs, "/project")
		require.NotNil(t, out.Definition)
		assert.Equal(t, "a.go", out.Definition.File)
	})
}

func TestBuildHoverJSON(t *testing.T) {
	t.Parallel()

	t.Run("nil hover", func(t *testing.T) {
		t.Parallel()
		out := buildHoverJSON(nil)
		assert.Equal(t, "", out.Hover)
	})

	t.Run("hover with content", func(t *testing.T) {
		t.Parallel()
		hover := &lsp.HoverResult{
			Contents: lsp.MarkupContent{Kind: "markdown", Value: "func Foo() error"},
		}
		out := buildHoverJSON(hover)
		assert.Equal(t, "func Foo() error", out.Hover)
	})
}

func TestBuildSymbolsJSON(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		out := buildSymbolsJSON(nil, "/project/main.go")
		assert.Empty(t, out.Symbols)
	})

	t.Run("flat symbols", func(t *testing.T) {
		t.Parallel()
		syms := []lsp.DocumentSymbol{
			{Name: "main", Kind: lsp.SymbolKindFunction, Range: lsp.Range{Start: lsp.Position{Line: 9, Character: 0}}},
			{Name: "run", Kind: lsp.SymbolKindFunction, Range: lsp.Range{Start: lsp.Position{Line: 19, Character: 0}}},
		}
		out := buildSymbolsJSON(syms, "/project/main.go")
		require.Len(t, out.Symbols, 2)
		assert.Equal(t, "main", out.Symbols[0].Name)
		assert.Equal(t, "function", out.Symbols[0].Kind)
		assert.Equal(t, 10, out.Symbols[0].Line)
		assert.Equal(t, "/project/main.go", out.Symbols[0].File)
	})

	t.Run("nested children flattened", func(t *testing.T) {
		t.Parallel()
		syms := []lsp.DocumentSymbol{
			{
				Name: "MyStruct", Kind: lsp.SymbolKindStruct,
				Range: lsp.Range{Start: lsp.Position{Line: 4}},
				Children: []lsp.DocumentSymbol{
					{Name: "Field", Kind: lsp.SymbolKindField, Range: lsp.Range{Start: lsp.Position{Line: 5}}},
				},
			},
		}
		out := buildSymbolsJSON(syms, "/project/types.go")
		require.Len(t, out.Symbols, 2)
		assert.Equal(t, "MyStruct", out.Symbols[0].Name)
		assert.Equal(t, "Field", out.Symbols[1].Name)
	})
}

func TestJSONShapeRoundtrip(t *testing.T) {
	t.Parallel()

	t.Run("refs JSON roundtrip", func(t *testing.T) {
		t.Parallel()
		locs := []lsp.Location{
			{URI: "file:///project/foo.go", Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}}},
		}
		out := buildRefsJSON(locs, "/project")
		data, err := json.Marshal(out)
		require.NoError(t, err)

		var decoded jsonRefsOutput
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Len(t, decoded.References, 1)
		assert.Equal(t, "foo.go", decoded.References[0].File)
		assert.Equal(t, 1, decoded.References[0].Line)
		assert.Equal(t, 1, decoded.References[0].Column)
	})
}

func TestResolveURIToRel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uri  string
		cwd  string
		want string
	}{
		{"file:///project/main.go", "/project", "main.go"},
		{"file:///project/pkg/server.go", "/project", "pkg/server.go"},
		{"file:///other/main.go", "/project", "../other/main.go"},
		{"file:///project/main.go", "", "/project/main.go"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.uri, func(t *testing.T) {
			t.Parallel()
			got := resolveURIToRel(tt.uri, tt.cwd)
			assert.Equal(t, tt.want, got)
		})
	}
}
