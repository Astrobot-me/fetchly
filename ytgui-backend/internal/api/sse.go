package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type event struct {
	name string
	data []byte
}

// Hub fans events out to every connected browser tab.
type Hub struct {
	mu   sync.Mutex
	subs map[chan event]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan event]struct{}{}} }

// Publish sends an event to all subscribers. A tab that falls too far
// behind is dropped; it reconnects and gets a fresh snapshot.
func (h *Hub) Publish(name string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- event{name, b}:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *Hub) subscribe() chan event {
	ch := make(chan event, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// CloseAll ends every stream (used on quit so tabs show "App stopped").
func (h *Hub) CloseAll() {
	h.mu.Lock()
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	write := func(name string, data []byte) error {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
			return err
		}
		return rc.Flush()
	}
	hello, _ := json.Marshal(map[string]any{"jobs": s.jobs.Snapshot(), "tools": s.toolsResponse()})
	if write("hello", hello) != nil {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.quitting:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if write(ev.name, ev.data) != nil {
				return
			}
		case <-ping.C:
			if write("ping", []byte("{}")) != nil {
				return
			}
		}
	}
}
