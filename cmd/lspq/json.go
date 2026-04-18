package main

import (
	"encoding/json"
	"os"

	"github.com/dotcommander/piglet/extensions/lsp"
)

type jsonLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type jsonDefOutput struct {
	Definition *jsonLocation `json:"definition"`
}

type jsonRefsOutput struct {
	References []jsonLocation `json:"references"`
}

type jsonHoverOutput struct {
	Hover string `json:"hover"`
}

type jsonSymbol struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type jsonSymbolsOutput struct {
	Symbols []jsonSymbol `json:"symbols"`
}

func writeJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

func buildDefJSON(locs []lsp.Location, cwd string) jsonDefOutput {
	if len(locs) == 0 {
		return jsonDefOutput{Definition: nil}
	}
	loc := locs[0]
	return jsonDefOutput{Definition: &jsonLocation{
		File:   resolveURIToRel(loc.URI, cwd),
		Line:   loc.Range.Start.Line + 1,
		Column: loc.Range.Start.Character + 1,
	}}
}

func buildRefsJSON(locs []lsp.Location, cwd string) jsonRefsOutput {
	out := jsonRefsOutput{References: make([]jsonLocation, 0, len(locs))}
	for _, loc := range locs {
		out.References = append(out.References, jsonLocation{
			File:   resolveURIToRel(loc.URI, cwd),
			Line:   loc.Range.Start.Line + 1,
			Column: loc.Range.Start.Character + 1,
		})
	}
	return out
}

func buildHoverJSON(hover *lsp.HoverResult) jsonHoverOutput {
	if hover == nil {
		return jsonHoverOutput{}
	}
	return jsonHoverOutput{Hover: hover.Contents.Value}
}

func buildSymbolsJSON(syms []lsp.DocumentSymbol, file string) jsonSymbolsOutput {
	out := jsonSymbolsOutput{Symbols: make([]jsonSymbol, 0, len(syms))}
	flattenSymbols(&out.Symbols, syms, file)
	return out
}

func flattenSymbols(dst *[]jsonSymbol, syms []lsp.DocumentSymbol, file string) {
	for _, s := range syms {
		*dst = append(*dst, jsonSymbol{
			Name:   s.Name,
			Kind:   s.Kind.String(),
			File:   file,
			Line:   s.Range.Start.Line + 1,
			Column: s.Range.Start.Character + 1,
		})
		if len(s.Children) > 0 {
			flattenSymbols(dst, s.Children, file)
		}
	}
}
