package lsp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// roundTrip marshals v to JSON, then unmarshals into a new value of the same
// type and returns it. Fails the test on any error.
func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// TestSymbolKindString verifies every named constant maps to its human name
// and that an out-of-range value returns "unknown".
func TestSymbolKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind SymbolKind
		want string
	}{
		{SymbolKindFile, "file"},
		{SymbolKindModule, "module"},
		{SymbolKindNamespace, "namespace"},
		{SymbolKindPackage, "package"},
		{SymbolKindClass, "class"},
		{SymbolKindMethod, "method"},
		{SymbolKindProperty, "property"},
		{SymbolKindField, "field"},
		{SymbolKindConstructor, "constructor"},
		{SymbolKindEnum, "enum"},
		{SymbolKindInterface, "interface"},
		{SymbolKindFunction, "function"},
		{SymbolKindVariable, "variable"},
		{SymbolKindConstant, "constant"},
		{SymbolKindString, "string"},
		{SymbolKindNumber, "number"},
		{SymbolKindBoolean, "boolean"},
		{SymbolKindArray, "array"},
		{SymbolKindObject, "object"},
		{SymbolKindKey, "key"},
		{SymbolKindNull, "null"},
		{SymbolKindEnumMember, "enum member"},
		{SymbolKindStruct, "struct"},
		{SymbolKindEvent, "event"},
		{SymbolKindOperator, "operator"},
		{SymbolKindTypeParameter, "type parameter"},
		{SymbolKind(0), "unknown"},
		{SymbolKind(999), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.kind.String())
		})
	}
}

// TestPositionRoundTrip verifies JSON marshaling/unmarshaling of Position.
func TestPositionRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []Position{
		{Line: 0, Character: 0},
		{Line: 10, Character: 42},
		{}, // zero value
	}

	for _, orig := range cases {
		got := roundTrip(t, orig)
		require.Equal(t, orig, got)
	}
}

// TestRangeRoundTrip verifies JSON round-trip for Range.
func TestRangeRoundTrip(t *testing.T) {
	t.Parallel()

	orig := Range{
		Start: Position{Line: 1, Character: 5},
		End:   Position{Line: 1, Character: 15},
	}
	require.Equal(t, orig, roundTrip(t, orig))
}

// TestLocationRoundTrip verifies JSON round-trip for Location.
func TestLocationRoundTrip(t *testing.T) {
	t.Parallel()

	orig := Location{
		URI: "file:///tmp/foo.go",
		Range: Range{
			Start: Position{Line: 3, Character: 0},
			End:   Position{Line: 3, Character: 10},
		},
	}
	require.Equal(t, orig, roundTrip(t, orig))
}

// TestLocationLinkRoundTrip verifies JSON round-trip for LocationLink.
func TestLocationLinkRoundTrip(t *testing.T) {
	t.Parallel()

	orig := LocationLink{
		TargetURI: "file:///tmp/bar.go",
		TargetRange: Range{
			Start: Position{Line: 7, Character: 2},
			End:   Position{Line: 9, Character: 4},
		},
	}
	require.Equal(t, orig, roundTrip(t, orig))
}

// TestTextDocumentItemRoundTrip verifies all fields survive round-trip.
func TestTextDocumentItemRoundTrip(t *testing.T) {
	t.Parallel()

	orig := TextDocumentItem{
		URI:        "file:///tmp/main.go",
		LanguageID: "go",
		Version:    3,
		Text:       "package main\n",
	}
	require.Equal(t, orig, roundTrip(t, orig))
}

// TestHoverResultOmitEmpty verifies that HoverResult.Range is omitted when nil.
func TestHoverResultOmitEmpty(t *testing.T) {
	t.Parallel()

	// Range nil: field should be absent from JSON
	h := HoverResult{Contents: MarkupContent{Kind: "plaintext", Value: "doc"}}
	data, err := json.Marshal(h)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	_, rangePresent := m["range"]
	require.False(t, rangePresent, "range field should be omitted when nil")

	// Range non-nil: field should appear
	r := Range{Start: Position{1, 0}, End: Position{1, 5}}
	h2 := HoverResult{Contents: MarkupContent{Kind: "markdown", Value: "**bold**"}, Range: &r}
	got := roundTrip(t, h2)
	require.NotNil(t, got.Range)
	require.Equal(t, r, *got.Range)
}

// TestWorkspaceEditOmitEmpty verifies that Changes is omitted when nil/empty.
func TestWorkspaceEditOmitEmpty(t *testing.T) {
	t.Parallel()

	// nil Changes: key should be absent
	empty := WorkspaceEdit{}
	data, err := json.Marshal(empty)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	_, ok := m["changes"]
	require.False(t, ok, "changes should be omitted when nil")

	// Populated Changes: round-trip should preserve all edits
	orig := WorkspaceEdit{
		Changes: map[string][]TextEdit{
			"file:///a.go": {
				{Range: Range{Start: Position{0, 0}, End: Position{0, 5}}, NewText: "hello"},
			},
		},
	}
	got := roundTrip(t, orig)
	require.Equal(t, orig, got)
}

// TestDocumentSymbolOptionalChildren verifies DocumentSymbol omits children
// field when empty and preserves it when populated.
func TestDocumentSymbolOptionalChildren(t *testing.T) {
	t.Parallel()

	// No children: JSON should not contain "children" key
	leaf := DocumentSymbol{
		Name:           "Foo",
		Kind:           SymbolKindFunction,
		Range:          Range{Start: Position{1, 0}, End: Position{5, 1}},
		SelectionRange: Range{Start: Position{1, 5}, End: Position{1, 8}},
	}
	data, err := json.Marshal(leaf)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	_, present := m["children"]
	require.False(t, present, "children should be omitted when empty")

	// With children: round-trip should preserve nesting
	child := DocumentSymbol{
		Name:           "Bar",
		Kind:           SymbolKindMethod,
		Range:          Range{Start: Position{2, 0}, End: Position{3, 1}},
		SelectionRange: Range{Start: Position{2, 5}, End: Position{2, 8}},
	}
	parent := DocumentSymbol{
		Name:           "MyStruct",
		Kind:           SymbolKindStruct,
		Range:          Range{Start: Position{1, 0}, End: Position{10, 1}},
		SelectionRange: Range{Start: Position{1, 7}, End: Position{1, 15}},
		Children:       []DocumentSymbol{child},
	}
	got := roundTrip(t, parent)
	require.Equal(t, parent, got)
}

// TestInitializeParamsOmitProcessID verifies ProcessID is omitted when zero.
func TestInitializeParamsOmitProcessID(t *testing.T) {
	t.Parallel()

	p := InitializeParams{RootURI: "file:///workspace"}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	_, ok := m["processId"]
	require.False(t, ok, "processId should be omitted when zero")

	// Non-zero pid should appear
	p2 := InitializeParams{ProcessID: 12345, RootURI: "file:///workspace"}
	got := roundTrip(t, p2)
	require.Equal(t, 12345, got.ProcessID)
}

// TestReferenceParamsEmbedFlattens verifies that the embedded
// TextDocumentPositionParams fields are marshaled at the top level.
func TestReferenceParamsEmbedFlattens(t *testing.T) {
	t.Parallel()

	orig := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: "file:///x.go"},
			Position:     Position{Line: 5, Character: 3},
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	// Embedded fields must appear at the top level
	require.Contains(t, m, "textDocument")
	require.Contains(t, m, "position")
	require.Contains(t, m, "context")

	got := roundTrip(t, orig)
	require.Equal(t, orig, got)
}

// TestRenameParamsEmbedFlattens mirrors the above for RenameParams.
func TestRenameParamsEmbedFlattens(t *testing.T) {
	t.Parallel()

	orig := RenameParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: "file:///y.go"},
			Position:     Position{Line: 2, Character: 7},
		},
		NewName: "myNewFunc",
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	require.Contains(t, m, "textDocument")
	require.Contains(t, m, "position")
	require.Contains(t, m, "newName")

	got := roundTrip(t, orig)
	require.Equal(t, orig, got)
}

// TestServerCapabilitiesAnyFields verifies ServerCapabilities can hold
// bool, object, and nil for its "any" provider fields without data loss.
//
// omitempty on an "any" field omits only nil — not false. A server may
// legitimately advertise a capability as false (explicitly unsupported), so
// the field must remain present when set to false.
func TestServerCapabilitiesAnyFields(t *testing.T) {
	t.Parallel()

	t.Run("true provider is present", func(t *testing.T) {
		t.Parallel()
		sc := ServerCapabilities{DefinitionProvider: true}
		data, err := json.Marshal(sc)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		require.Equal(t, true, m["definitionProvider"])
	})

	t.Run("nil provider is omitted", func(t *testing.T) {
		t.Parallel()
		// All fields left nil — omitempty must drop them entirely.
		sc := ServerCapabilities{}
		data, err := json.Marshal(sc)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		require.Empty(t, m, "nil any fields must be omitted by omitempty")
	})

	t.Run("false provider is present (any != bool omitempty)", func(t *testing.T) {
		t.Parallel()
		// When an "any" field holds false, it is non-nil so omitempty keeps it.
		sc := ServerCapabilities{HoverProvider: false}
		data, err := json.Marshal(sc)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		_, ok := m["hoverProvider"]
		require.True(t, ok, "hoverProvider=false (non-nil any) should appear in JSON")
	})

	t.Run("struct object provider survives round-trip", func(t *testing.T) {
		t.Parallel()
		type options struct {
			WorkDoneProgress bool `json:"workDoneProgress"`
		}
		sc := ServerCapabilities{
			ReferencesProvider: options{WorkDoneProgress: true},
		}
		data, err := json.Marshal(sc)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		prov, ok := m["referencesProvider"]
		require.True(t, ok)
		require.IsType(t, map[string]any{}, prov)
	})
}

// TestSymbolInformationRoundTrip exercises the flat symbol format.
func TestSymbolInformationRoundTrip(t *testing.T) {
	t.Parallel()

	orig := SymbolInformation{
		Name: "Handler",
		Kind: SymbolKindFunction,
		Location: Location{
			URI: "file:///handler.go",
			Range: Range{
				Start: Position{20, 0},
				End:   Position{30, 1},
			},
		},
	}
	require.Equal(t, orig, roundTrip(t, orig))
}

// TestZeroValueTypes verifies that zero-value structs unmarshal cleanly from
// an empty JSON object and produce sensible (usable) defaults.
func TestZeroValueTypes(t *testing.T) {
	t.Parallel()

	unmarshalEmpty := func(t *testing.T, v any) {
		t.Helper()
		require.NoError(t, json.Unmarshal([]byte("{}"), v))
	}

	t.Run("Position", func(t *testing.T) {
		t.Parallel()
		var v Position
		unmarshalEmpty(t, &v)
		require.Equal(t, Position{}, v)
	})

	t.Run("Range", func(t *testing.T) {
		t.Parallel()
		var v Range
		unmarshalEmpty(t, &v)
		require.Equal(t, Range{}, v)
	})

	t.Run("HoverResult", func(t *testing.T) {
		t.Parallel()
		var v HoverResult
		unmarshalEmpty(t, &v)
		require.Nil(t, v.Range)
		require.Equal(t, "", v.Contents.Value)
	})

	t.Run("WorkspaceEdit", func(t *testing.T) {
		t.Parallel()
		var v WorkspaceEdit
		unmarshalEmpty(t, &v)
		require.Nil(t, v.Changes)
	})

	t.Run("DocumentSymbol", func(t *testing.T) {
		t.Parallel()
		var v DocumentSymbol
		unmarshalEmpty(t, &v)
		require.Nil(t, v.Children)
	})
}
