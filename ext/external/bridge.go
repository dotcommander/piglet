package external

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/dotcommander/piglet/core"
	"github.com/dotcommander/piglet/ext"
)

// LoadAll discovers and starts all external extensions under supervisor management,
// registering their tools, commands, and prompt sections with the given ext.App.
// projectDir, if non-empty, is scanned for additional extensions (opt-in via config).
// Returns the number of loaded extensions and a cleanup function that stops all.
func LoadAll(ctx context.Context, app *ext.App, undoFn UndoSnapshotsFn, disabled []string, projectDir string) (loaded int, cleanup func(), err error) {
	extDir, err := ExtensionsDir()
	if err != nil {
		return 0, func() {}, nil // non-fatal
	}

	manifests, err := DiscoverExtensions(extDir)
	if err != nil {
		return 0, func() {}, nil // non-fatal
	}

	// Discover project-local extensions (override global by name)
	if projectDir != "" {
		projectManifests, pErr := DiscoverExtensions(projectDir)
		if pErr != nil {
			slog.Warn("project extensions discovery failed", "dir", projectDir, "err", pErr)
		} else if len(projectManifests) > 0 {
			slog.Info("discovered project-local extensions", "dir", projectDir, "count", len(projectManifests))
			projectNames := make(map[string]bool, len(projectManifests))
			for _, pm := range projectManifests {
				projectNames[pm.Name] = true
			}
			// Remove global manifests that project-local overrides
			manifests = slices.DeleteFunc(manifests, func(m *Manifest) bool { return projectNames[m.Name] })
			manifests = append(manifests, projectManifests...)
		}
	}

	// Filter out disabled extensions
	if len(disabled) > 0 {
		filtered := make([]*Manifest, 0, len(manifests))
		for _, m := range manifests {
			if slices.Contains(disabled, m.Name) {
				slog.Info("extension disabled by config", "name", m.Name)
				continue
			}
			filtered = append(filtered, m)
		}
		manifests = filtered
	}

	if len(manifests) == 0 {
		return 0, func() {}, nil
	}

	resolverFn := makeProviderResolver()

	// Start all extensions concurrently (each blocks on handshake)
	type result struct {
		sup *Supervisor
		err error
	}
	results := make([]result, len(manifests))
	var wg sync.WaitGroup
	loadStart := time.Now()
	for i, m := range manifests {
		wg.Add(1)
		go func(i int, m *Manifest) {
			defer wg.Done()
			s := NewSupervisor(m, app.CWD(), app, resolverFn, undoFn)
			if err := s.Start(ctx); err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{sup: s}
		}(i, m)
	}
	wg.Wait()
	slog.Debug("extensions loaded", "count", len(manifests), "elapsed", time.Since(loadStart).Round(time.Millisecond))

	var supervisors []*Supervisor
	for i, r := range results {
		if r.err != nil {
			slog.Warn("failed to start extension", "name", manifests[i].Name, "err", r.err)
			continue
		}
		supervisors = append(supervisors, r.sup)
	}

	// Start hot-reload watcher for extension binaries/manifests.
	watcher := startWatcher(supervisors)

	return len(supervisors), func() {
		watcher.Stop() // safe on nil
		var wg sync.WaitGroup
		for _, s := range supervisors {
			wg.Add(1)
			go func(s *Supervisor) {
				defer wg.Done()
				s.Stop()
			}(s)
		}
		wg.Wait()
	}, nil
}

// bridge wires a single host's registrations into ext.App.
func bridge(app *ext.App, h *Host) {
	h.bridge(app)
}

func (h *Host) bridge(app *ext.App) {
	info := ext.ExtInfo{
		Name:    h.Name(),
		Kind:    "external",
		Runtime: h.manifest.Runtime,
		Version: h.manifest.Version,
	}
	h.bridgeTools(app, &info)
	h.bridgeCommands(app, &info)
	h.bridgePromptSections(app, &info)
	h.bridgeInterceptors(app, &info)
	h.bridgeEventHandlers(app, &info)
	h.bridgeShortcuts(app, &info)
	h.bridgeMessageHooks(app, &info)
	h.bridgeInputTransformers(app, &info)
	h.bridgeCompactor(app, &info)
	h.bridgeStreamProviders(app, &info)
	app.RegisterExtInfo(info)
}

func (h *Host) bridgeTools(app *ext.App, info *ext.ExtInfo) {
	for _, t := range h.Tools() {
		def := &ext.ToolDef{
			ToolSchema: core.ToolSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
			PromptHint: t.PromptHint,
			Deferred:   t.Deferred,
			Execute:    proxyToolExecute(h, t.Name),
		}
		if t.InterruptBehavior == InterruptBehaviorBlock {
			def.InterruptBehavior = ext.InterruptBlock
		}
		app.RegisterTool(def)
		info.Tools = append(info.Tools, t.Name)
	}
}

func (h *Host) bridgeCommands(app *ext.App, info *ext.ExtInfo) {
	for _, c := range h.Commands() {
		app.RegisterCommand(&ext.Command{
			Name:        c.Name,
			Description: c.Description,
			Immediate:   c.Immediate,
			Handler:     proxyCommandExecute(h, c.Name),
		})
		info.Commands = append(info.Commands, c.Name)
	}
}

func (h *Host) bridgePromptSections(app *ext.App, info *ext.ExtInfo) {
	for _, ps := range h.PromptSections() {
		app.RegisterPromptSection(ext.PromptSection{
			Title:     ps.Title,
			Content:   ps.Content,
			Order:     ps.Order,
			TokenHint: ps.TokenHint,
		})
		info.PromptSections = append(info.PromptSections, ps.Title)
	}
}

func (h *Host) bridgeInterceptors(app *ext.App, info *ext.ExtInfo) {
	for _, ic := range h.Interceptors() {
		before, preview := proxyInterceptorBeforeWithPreview(h, ic.Name)
		app.RegisterInterceptor(ext.Interceptor{
			Name:     ic.Name,
			Priority: ic.Priority,
			Before:   before,
			After:    proxyInterceptorAfter(h, ic.Name),
			Preview:  preview,
		})
		info.Interceptors = append(info.Interceptors, ic.Name)
	}
}

func (h *Host) bridgeEventHandlers(app *ext.App, info *ext.ExtInfo) {
	for _, eh := range h.EventHandlers() {
		app.RegisterEventHandler(ext.EventHandler{
			Name:     eh.Name,
			Priority: eh.Priority,
			Filter:   proxyEventFilter(eh.Events),
			Handle:   proxyEventHandle(h),
		})
		info.EventHandlers = append(info.EventHandlers, eh.Name)
	}
}

func (h *Host) bridgeShortcuts(app *ext.App, info *ext.ExtInfo) {
	for _, sc := range h.Shortcuts() {
		app.RegisterShortcut(&ext.Shortcut{
			Key:         sc.Key,
			Description: sc.Description,
			Handler:     proxyShortcutHandle(h, sc.Key),
		})
		info.Shortcuts = append(info.Shortcuts, sc.Key)
	}
}

func (h *Host) bridgeMessageHooks(app *ext.App, info *ext.ExtInfo) {
	for _, mh := range h.MessageHooks() {
		app.RegisterMessageHook(ext.MessageHook{
			Name:      mh.Name,
			Priority:  mh.Priority,
			OnMessage: proxyMessageHook(h),
		})
		info.MessageHooks = append(info.MessageHooks, mh.Name)
	}
}

func (h *Host) bridgeInputTransformers(app *ext.App, info *ext.ExtInfo) {
	for _, it := range h.InputTransformers() {
		app.RegisterInputTransformer(ext.InputTransformer{
			Name:      it.Name,
			Priority:  it.Priority,
			Transform: proxyInputTransform(h),
		})
		info.InputTransformers = append(info.InputTransformers, it.Name)
	}
}

func (h *Host) bridgeCompactor(app *ext.App, info *ext.ExtInfo) {
	cp := h.Compactor()
	if cp == nil {
		return
	}
	app.RegisterCompactor(ext.Compactor{
		Name:      cp.Name,
		Threshold: cp.Threshold,
		Compact:   proxyCompactExecute(h),
	})
	info.Compactor = cp.Name
}

func (h *Host) bridgeStreamProviders(app *ext.App, info *ext.ExtInfo) {
	for _, p := range h.Providers() {
		api := p.API
		app.RegisterStreamProvider(api, func(model core.Model) core.StreamProvider {
			return &proxyStreamProvider{host: h, model: model}
		})
		info.StreamProviders = append(info.StreamProviders, api)
	}
}
