// Package realtime is a tiny in-memory pub/sub hub that fans events out to
// live subscribers (SSE streams and WebTransport datagram sessions).
//
// Topics are arbitrary strings. We use two namespaces:
//   - a group id            -> events for everyone viewing that group
//   - "user:" + a user id   -> personal push notifications for one user
package realtime

import "sync"

// Event is a single broadcastable update.
type Event struct {
	Topic   string `json:"topic"`
	Kind    string `json:"kind"` // e.g. "expense", "settlement", "member", "comment"
	Message string `json:"message"`
}

// UserTopic returns the per-user push-notification topic for a user id.
func UserTopic(userID string) string { return "user:" + userID }

// Hub manages subscribers keyed by topic.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan Event]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a channel for a topic and returns an unsubscribe func.
// The channel is buffered so a slow client never blocks the publisher.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[chan Event]struct{})
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if set, ok := h.subs[topic]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(h.subs, topic)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
}

// Publish broadcasts an event to every subscriber of a topic.
// Drops the event for any subscriber whose buffer is full (best-effort).
func (h *Hub) Publish(topic string, e Event) {
	e.Topic = topic
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[topic] {
		select {
		case ch <- e:
		default: // slow consumer: skip rather than stall the hub
		}
	}
}
