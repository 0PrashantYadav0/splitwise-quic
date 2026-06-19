package handlers

import (
	"fmt"
	"log"
	"net/http"

	"splitwise-quic/internal/realtime"
)

// sse streams group events to the browser via Server-Sent Events.
// HTMX's SSE extension consumes these and triggers partial refreshes.
// Works over both HTTP/2 (TCP) and HTTP/3 (QUIC) transparently.
func (h *Handlers) sse(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, unsub := h.hub.Subscribe(g.ID)
	defer unsub()

	// Initial comment so the client marks the stream open immediately.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-events:
			fmt.Fprintf(w, "event: update\ndata: %s\n\n", ev.Message)
			flusher.Flush()
		}
	}
}

// webTransport upgrades a QUIC connection to a WebTransport session and
// pushes group events as unreliable QUIC DATAGRAM frames - the lowest-latency
// live channel available to browsers today.
func (h *Handlers) webTransport(w http.ResponseWriter, r *http.Request) {
	// Auth + membership check (no middleware wraps this endpoint).
	u := h.currentUser(r)
	if u == nil {
		httpError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	gid := r.PathValue("id")
	ok, err := h.store.IsMember(gid, u.ID)
	if err != nil || !ok {
		httpError(w, "forbidden", http.StatusForbidden)
		return
	}

	session, err := h.srv.WebTransport().Upgrade(w, r)
	if err != nil {
		log.Printf("webtransport upgrade failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer session.CloseWithError(0, "bye")

	events, unsub := h.hub.Subscribe(gid)
	defer unsub()

	// Greet the client so the JS flips the live indicator on.
	_ = session.SendDatagram([]byte("live channel connected"))

	for {
		select {
		case <-session.Context().Done():
			return
		case ev := <-events:
			if err := session.SendDatagram([]byte(ev.Message)); err != nil {
				return // session gone; client will auto-reconnect
			}
		}
	}
}

// userWebTransport opens a per-user push channel. It is connected from every
// page so a user receives personal notifications (added to a group, a new
// expense that involves them, a settlement) as QUIC datagrams anywhere in
// the app - not just while viewing one group.
func (h *Handlers) userWebTransport(w http.ResponseWriter, r *http.Request) {
	u := h.currentUser(r)
	if u == nil {
		httpError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := h.srv.WebTransport().Upgrade(w, r)
	if err != nil {
		log.Printf("user webtransport upgrade failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer session.CloseWithError(0, "bye")

	events, unsub := h.hub.Subscribe(realtime.UserTopic(u.ID))
	defer unsub()

	_ = session.SendDatagram([]byte("notifications connected"))

	for {
		select {
		case <-session.Context().Done():
			return
		case ev := <-events:
			if err := session.SendDatagram([]byte(ev.Message)); err != nil {
				return
			}
		}
	}
}
