package ext

import "slices"

// UnregisterExtension removes all registrations associated with the named extension.
// Used by the supervisor when restarting a crashed extension process.
func (a *App) UnregisterExtension(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Find the extension's info to know what was registered
	var info *ExtInfo
	for i := range a.extInfos {
		if a.extInfos[i].Name == name {
			info = &a.extInfos[i]
			break
		}
	}
	if info == nil {
		return
	}

	// Remove tools
	for _, t := range info.Tools {
		delete(a.tools, t)
	}

	// Remove commands
	for _, c := range info.Commands {
		delete(a.commands, c)
	}

	// Remove shortcuts
	for _, k := range info.Shortcuts {
		delete(a.shortcuts, k)
	}

	nameSet := func(names []string) map[string]bool {
		m := make(map[string]bool, len(names))
		for _, n := range names {
			m[n] = true
		}
		return m
	}

	interceptorNames := nameSet(info.Interceptors)
	handlerNames := nameSet(info.EventHandlers)
	hookNames := nameSet(info.MessageHooks)
	transformerNames := nameSet(info.InputTransformers)
	sectionTitles := nameSet(info.PromptSections)

	a.interceptors = slices.DeleteFunc(a.interceptors, func(ic Interceptor) bool {
		return interceptorNames[ic.Name]
	})
	a.eventHandlers = slices.DeleteFunc(a.eventHandlers, func(eh EventHandler) bool {
		return handlerNames[eh.Name]
	})
	a.messageHooks = slices.DeleteFunc(a.messageHooks, func(mh MessageHook) bool {
		return hookNames[mh.Name]
	})
	a.inputTransformers = slices.DeleteFunc(a.inputTransformers, func(it InputTransformer) bool {
		return transformerNames[it.Name]
	})
	a.promptSections = slices.DeleteFunc(a.promptSections, func(ps PromptSection) bool {
		return sectionTitles[ps.Title]
	})

	// Remove compactor if it belongs to this extension
	if a.compactor != nil && info.Compactor != "" && a.compactor.Name == info.Compactor {
		a.compactor = nil
	}

	// Remove stream providers
	for _, api := range info.StreamProviders {
		delete(a.streamProviders, api)
	}

	// Remove the ext info entry itself
	a.extInfos = slices.DeleteFunc(a.extInfos, func(ei ExtInfo) bool {
		return ei.Name == name
	})
}
