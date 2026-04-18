package sdk

import (
	"context"
	"encoding/json"
)

// Publish sends data to all subscribers of an event bus topic.
func (e *Extension) Publish(ctx context.Context, topic string, data any) error {
	return hostCallVoid(e, ctx, "host/publish", map[string]any{
		"topic": topic,
		"data":  data,
	})
}

// Subscription tracks an event bus subscription for cleanup.
type Subscription struct {
	ID    int
	topic string
	ext   *Extension
	ch    chan json.RawMessage
}

// Events returns a channel that receives events for this subscription.
func (s *Subscription) Events() <-chan json.RawMessage { return s.ch }

// Subscribe registers for events on a topic. Returns a Subscription whose
// Events() channel receives data whenever the topic fires. Call Unsubscribe()
// when done. Events are delivered as json.RawMessage — unmarshal to your type.
func (e *Extension) Subscribe(ctx context.Context, topic string) (*Subscription, error) {
	type resp struct {
		SubscriptionID int `json:"subscriptionId"`
	}
	r, err := hostCall[resp](e, ctx, "host/subscribe", map[string]any{"topic": topic})
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		ID:    r.SubscriptionID,
		topic: topic,
		ext:   e,
		ch:    make(chan json.RawMessage, 64),
	}

	e.subsMu.Lock()
	e.subs[sub.ID] = sub
	e.subsMu.Unlock()

	return sub, nil
}

// Unsubscribe removes the subscription. The Events() channel is closed.
func (s *Subscription) Unsubscribe() {
	s.ext.subsMu.Lock()
	delete(s.ext.subs, s.ID)
	s.ext.subsMu.Unlock()
	close(s.ch)
}
