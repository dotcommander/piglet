package sdk

// sendRegistrations sends all extension registrations to the host as notifications.
func (e *Extension) sendRegistrations() {
	for _, t := range e.tools {
		params := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
			"promptHint":  t.PromptHint,
			"deferred":    t.Deferred,
		}
		if t.InterruptBehavior != "" {
			params["interruptBehavior"] = t.InterruptBehavior
		}
		e.sendNotification("register/tool", params)
	}
	for _, c := range e.commands {
		params := map[string]any{
			"name":        c.Name,
			"description": c.Description,
		}
		if c.Immediate {
			params["immediate"] = true
		}
		e.sendNotification("register/command", params)
	}
	for _, s := range e.promptSections {
		params := map[string]any{
			"title":   s.Title,
			"content": s.Content,
			"order":   s.Order,
		}
		if s.TokenHint > 0 {
			params["tokenHint"] = s.TokenHint
		}
		e.sendNotification("register/promptSection", params)
	}
	for _, i := range e.interceptors {
		e.sendNotification("register/interceptor", map[string]any{
			"name":     i.Name,
			"priority": i.Priority,
		})
	}
	for _, h := range e.eventHandlers {
		e.sendNotification("register/eventHandler", map[string]any{
			"name":     h.Name,
			"priority": h.Priority,
			"events":   h.Events,
		})
	}
	for _, s := range e.shortcuts {
		e.sendNotification("register/shortcut", map[string]any{
			"key":         s.Key,
			"description": s.Description,
		})
	}
	for _, h := range e.messageHooks {
		e.sendNotification("register/messageHook", map[string]any{
			"name":     h.Name,
			"priority": h.Priority,
		})
	}
	for _, it := range e.inputTransformers {
		e.sendNotification("register/inputTransformer", map[string]any{
			"name":     it.Name,
			"priority": it.Priority,
		})
	}
	if e.compactor != nil {
		e.sendNotification("register/compactor", map[string]any{
			"name":      e.compactor.Name,
			"threshold": e.compactor.Threshold,
		})
	}
}
