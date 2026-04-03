package repomap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dotcommander/piglet/extensions/internal/xdg"
	sdk "github.com/dotcommander/piglet/sdk"
)

// codeChangingTools lists tool names that modify source code.
var codeChangingTools = map[string]bool{
	"write_file":    true,
	"edit_file":     true,
	"bash":          true,
	"notebook_edit": true,
	"multi_edit":    true,
}

// inventoryParams defines parameters for the repomap_inventory tool.
var inventoryParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        []string{"scan", "query"},
			"description": "scan: rebuild inventory from disk. query: filter existing inventory.",
		},
		"filter": map[string]any{
			"type":        "string",
			"description": "Filter expression for query (e.g. 'lines>100', 'path=internal/')",
		},
	},
	"required": []string{"action"},
}

// repomapToolParams is shared between repomap_show and repomap_refresh tools.
var repomapToolParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verbose": map[string]any{
			"type":        "boolean",
			"description": "Show all symbols grouped by category (default: false)",
		},
		"detail": map[string]any{
			"type":        "boolean",
			"description": "Show all symbols with full signatures (default: false)",
		},
	},
}

// Register wires the repomap extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	var (
		rm      *Map
		extRef  *sdk.Extension
		built   bool
		builtMu sync.RWMutex
	)

	setBuilt := func(v bool) {
		builtMu.Lock()
		built = v
		builtMu.Unlock()
	}

	isBuilt := func() bool {
		builtMu.RLock()
		defer builtMu.RUnlock()
		return built
	}

	buildInBackground := func() {
		extRef.Notify("Scanning repository...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		start := time.Now()
		if err := rm.Build(ctx); err != nil {
			if errors.Is(err, ErrNotCodeProject) {
				extRef.Log("debug", "skipping repomap: no source files found")
			} else {
				extRef.Notify("Scan failed")
				extRef.Log("warn", "repomap background build failed: "+err.Error())
			}
			return
		}

		elapsed := time.Since(start).Round(time.Millisecond)
		out := rm.StringLines()
		if out == "" {
			extRef.Notify("No source files found")
			extRef.Log("warn", "repomap produced empty output")
			setBuilt(true)
			return
		}

		setBuilt(true)
		extRef.Notify("Map ready")
		extRef.Log("info", "repomap built in "+elapsed.String())
	}

	e.OnInit(func(x *sdk.Extension) {
		start := time.Now()
		x.Log("debug", "[repomap] OnInit start")

		extRef = x

		cachedInv := LoadInventory(repomapCacheDir())
		if cachedInv != nil {
			x.Log("debug", fmt.Sprintf("[repomap] inventory cache found: %d files", len(cachedInv.Files)))
		}
		cfg := loadRepomapConfig()
		rm = New(x.CWD(), cfg)

		cd := repomapCacheDir()
		rm.SetCacheDir(cd)

		// Try disk cache first — instant startup
		if rm.LoadCache(cd) {
			x.Log("debug", fmt.Sprintf("[repomap] cache hit (%s)", time.Since(start)))
			setBuilt(true)
			x.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   "Repository Map",
				Content: rm.StringLines(),
				Order:   95,
			})
			go func() {
				if !rm.Stale() {
					return
				}
				buildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := rm.Build(buildCtx); err != nil {
					if !errors.Is(err, ErrNotCodeProject) {
						x.Log("warn", "repomap background rebuild: "+err.Error())
					}
				}
			}()
			x.Log("debug", fmt.Sprintf("[repomap] OnInit complete (%s)", time.Since(start)))
			return
		}

		x.Log("debug", fmt.Sprintf("[repomap] cache miss — quick build start (%s)", time.Since(start)))

		quickCtx, quickCancel := context.WithTimeout(context.Background(), 5*time.Second)
		buildErr := rm.Build(quickCtx)
		quickCancel()
		if buildErr == nil {
			x.Log("debug", fmt.Sprintf("[repomap] quick build done (%s)", time.Since(start)))
			setBuilt(true)
			x.RegisterPromptSection(sdk.PromptSectionDef{
				Title:   "Repository Map",
				Content: rm.StringLines(),
				Order:   95,
			})
			x.Log("debug", fmt.Sprintf("[repomap] OnInit complete (%s)", time.Since(start)))
			return
		}
		if errors.Is(buildErr, ErrNotCodeProject) {
			x.Log("debug", "skipping repomap: no source files found")
			x.Log("debug", fmt.Sprintf("[repomap] OnInit complete — not a code project (%s)", time.Since(start)))
			return
		}

		x.Log("debug", fmt.Sprintf("[repomap] quick build timed out — continuing in background (%s)", time.Since(start)))

		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Repository Map",
			Content: "",
			Order:   95,
		})
		x.Log("debug", fmt.Sprintf("[repomap] OnInit complete (%s)", time.Since(start)))
		go buildInBackground()
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "repomap-stale-check",
		Priority: 50,
		Events:   []string{"turn_end"},
		Handle: func(ctx context.Context, eventType string, data json.RawMessage) *sdk.Action {
			if rm == nil {
				return nil
			}

			if turnModifiedCode(data) {
				rm.Dirty()
			}

			if !rm.Stale() {
				return nil
			}
			go func() {
				buildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := rm.Build(buildCtx); err != nil {
					if !errors.Is(err, ErrNotCodeProject) {
						extRef.Log("warn", "repomap rebuild failed: "+err.Error())
					}
				}
			}()
			return nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_refresh",
		Description: "Force rebuild the repository map after major file changes.",
		Parameters:  repomapToolParams,
		PromptHint:  "Rebuild the repository map after major file changes",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if rm == nil {
				return sdk.ErrorResult("repository map not initialized"), nil
			}
			if err := rm.Build(ctx); err != nil {
				return sdk.ErrorResult("rebuild failed: " + err.Error()), nil
			}
			setBuilt(true)
			return sdk.TextResult(formatRepomapOutput(rm, args)), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_show",
		Description: "Show the current repository structure map with source code definitions.",
		Parameters:  repomapToolParams,
		PromptHint:  "Show the current repository structure map (default: source lines, verbose/detail for alternatives)",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if rm == nil {
				return sdk.TextResult("Repository map not initialized."), nil
			}
			out := formatRepomapOutput(rm, args)
			if out == "" {
				if !isBuilt() {
					return sdk.TextResult("Repository map is still building..."), nil
				}
				return sdk.TextResult("Repository map is empty (no source files found)."), nil
			}
			return sdk.TextResult(out), nil
		},
	})

	e.RegisterTool(sdk.ToolDef{
		Name:        "repomap_inventory",
		Description: "Scan repository files for metrics (lines, imports) and query the inventory.",
		Parameters:  inventoryParams,
		PromptHint:  "Query per-file metrics: lines, imports. Use 'scan' to build, 'query' to filter.",
		Execute: func(ctx context.Context, args map[string]any) (*sdk.ToolResult, error) {
			if extRef == nil {
				return sdk.TextResult("repomap not initialized"), nil
			}
			action, _ := args["action"].(string)
			filter, _ := args["filter"].(string)

			switch action {
			case "scan":
				inv, err := ScanInventory(ctx, extRef.CWD())
				if err != nil {
					return sdk.ErrorResult("scan failed: " + err.Error()), nil
				}
				if err := PersistInventory(inv, repomapCacheDir()); err != nil {
					extRef.Log("warn", "inventory persist failed: "+err.Error())
				}
				header := fmt.Sprintf("Inventory: %d files (scanned %s)\n\n", len(inv.Files), inv.Scanned)
				if inv.Truncated {
					header = fmt.Sprintf("Inventory: %d files — truncated at cap %d (scanned %s)\n\n", len(inv.Files), inventoryFileCap, inv.Scanned)
				}
				return sdk.TextResult(formatInventoryTable(inv.Files, header)), nil
			case "query":
				out, err := QueryInventory(repomapCacheDir(), filter)
				if err != nil {
					return sdk.ErrorResult(err.Error()), nil
				}
				return sdk.TextResult(out), nil
			default:
				return sdk.ErrorResult("unknown action: " + action + " (expected 'scan' or 'query')"), nil
			}
		},
	})
}

// formatRepomapOutput returns the repomap in the requested format.
func formatRepomapOutput(rm *Map, args map[string]any) string {
	verbose, _ := args["verbose"].(bool)
	detail, _ := args["detail"].(bool)
	switch {
	case detail:
		return rm.StringDetail()
	case verbose:
		return rm.StringVerbose()
	default:
		return rm.StringLines()
	}
}

// pigletConfig mirrors the relevant subset of ~/.config/piglet/config.yaml.
type pigletConfig struct {
	Repomap struct {
		MaxTokens      int `yaml:"maxTokens"`
		MaxTokensNoCtx int `yaml:"maxTokensNoCtx"`
	} `yaml:"repomap"`
}

// loadRepomapConfig reads repomap settings from ~/.config/piglet/config.yaml.
func loadRepomapConfig() Config {
	cfg := DefaultConfig()
	pc := xdg.LoadYAMLExt("repomap", "config.yaml", pigletConfig{})

	if pc.Repomap.MaxTokens > 0 {
		cfg.MaxTokens = pc.Repomap.MaxTokens
	}
	if pc.Repomap.MaxTokensNoCtx > 0 {
		cfg.MaxTokensNoCtx = pc.Repomap.MaxTokensNoCtx
	}

	return cfg
}

// repomapCacheDir returns the repomap cache directory.
func repomapCacheDir() string {
	configDir, err := xdg.ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "cache")
}

// turnModifiedCode checks if the turn's tool results include code-changing tools.
func turnModifiedCode(data json.RawMessage) bool {
	var payload struct {
		ToolResults []struct {
			ToolName string `json:"toolName"`
		} `json:"ToolResults"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	for _, tr := range payload.ToolResults {
		if codeChangingTools[tr.ToolName] {
			return true
		}
	}
	return false
}
