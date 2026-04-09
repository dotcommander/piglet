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

// initState holds mutable state shared between OnInit and event handlers.
type initState struct {
	rm      *Map
	extRef  *sdk.Extension
	built   bool
	builtMu sync.RWMutex
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

// handleOnInit is called once at extension initialization. It attempts a
// three-tier build strategy: disk cache (instant), quick build (5s timeout),
// then falls back to a background build.
func handleOnInit(x *sdk.Extension, s *initState) {
	start := time.Now()
	x.Log("debug", "[repomap] OnInit start")

	s.extRef = x

	cachedInv := LoadInventory(repomapCacheDir())
	if cachedInv != nil {
		x.Log("debug", fmt.Sprintf("[repomap] inventory cache found: %d files", len(cachedInv.Files)))
	}
	cfg := loadRepomapConfig()
	s.rm = New(x.CWD(), cfg)

	cd := repomapCacheDir()
	s.rm.SetCacheDir(cd)

	// Try disk cache first — instant startup
	if s.rm.LoadCache(cd) {
		x.Log("debug", fmt.Sprintf("[repomap] cache hit (%s)", time.Since(start)))
		s.setBuilt(true)
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Repository Map",
			Content: s.rm.StringLines(),
			Order:   95,
		})
		go func() {
			if !s.rm.Stale() {
				return
			}
			buildCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.rm.Build(buildCtx); err != nil {
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
	buildErr := s.rm.Build(quickCtx)
	quickCancel()
	if buildErr == nil {
		x.Log("debug", fmt.Sprintf("[repomap] quick build done (%s)", time.Since(start)))
		s.setBuilt(true)
		x.RegisterPromptSection(sdk.PromptSectionDef{
			Title:   "Repository Map",
			Content: s.rm.StringLines(),
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
	go s.buildInBackground()
}

func (s *initState) setBuilt(v bool) {
	s.builtMu.Lock()
	s.built = v
	s.builtMu.Unlock()
}

func (s *initState) isBuilt() bool {
	s.builtMu.RLock()
	defer s.builtMu.RUnlock()
	return s.built
}

func (s *initState) buildInBackground() {
	s.extRef.Notify("Scanning repository...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	if err := s.rm.Build(ctx); err != nil {
		if errors.Is(err, ErrNotCodeProject) {
			s.extRef.Log("debug", "skipping repomap: no source files found")
		} else {
			s.extRef.Notify("Scan failed")
			s.extRef.Log("warn", "repomap background build failed: "+err.Error())
		}
		return
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	out := s.rm.StringLines()
	if out == "" {
		s.extRef.Notify("No source files found")
		s.extRef.Log("warn", "repomap produced empty output")
		s.setBuilt(true)
		return
	}

	s.setBuilt(true)
	s.extRef.Notify("Map ready")
	s.extRef.Log("info", "repomap built in "+elapsed.String())
}
