package ext

import "slices"

// eventSub is an internal subscriber record for the inter-extension event bus.
type eventSub struct {
	id int
	fn func(any)
}

// Subscribe registers a callback for a topic. Returns an unsubscribe function.
// Callbacks run synchronously in the publisher's goroutine — keep them fast.
func (a *App) Subscribe(topic string, fn func(any)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.eventBusSeq++
	id := a.eventBusSeq
	a.eventBus[topic] = append(a.eventBus[topic], eventSub{id: id, fn: fn})
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.eventBus[topic] = slices.DeleteFunc(a.eventBus[topic], func(s eventSub) bool { return s.id == id })
	}
}

// Publish sends data to all subscribers of a topic.
// Callbacks run synchronously — keep them fast or use goroutines in the subscriber.
func (a *App) Publish(topic string, data any) {
	a.mu.RLock()
	subs := make([]eventSub, len(a.eventBus[topic]))
	copy(subs, a.eventBus[topic])
	a.mu.RUnlock()
	for _, sub := range subs {
		sub.fn(data)
	}
}
