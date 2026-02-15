package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Broadcaster fans out progress events to multiple SSE clients.
// One Broadcaster per pipeline run. Thread-safe.
type Broadcaster struct {
	mu      sync.Mutex
	history []map[string]any
	clients map[uint64]chan map[string]any
	nextID  uint64
	closed  bool
	doneCh  chan struct{} // closed only on real broadcaster Close(), not slow-client drops
}

// NewBroadcaster creates a new event broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[uint64]chan map[string]any),
		doneCh:  make(chan struct{}),
	}
}

// Send is the progressSink callback. Called by the engine for every progress event.
// The map is already a deep-copied snapshot (engine guarantees this).
func (b *Broadcaster) Send(ev map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.history = append(b.history, ev)
	for id, ch := range b.clients {
		select {
		case ch <- ev:
		default:
			// Slow client: drop to prevent blocking the engine.
			close(ch)
			delete(b.clients, id)
		}
	}
}

// Subscribe returns an events channel, a done channel, and an unsubscribe function.
// The events channel receives a replay of all historical events, then live events.
// The done channel is closed only when the broadcaster is closed (pipeline finished),
// NOT when a slow client is dropped. This lets callers distinguish the two cases.
func (b *Broadcaster) Subscribe() (<-chan map[string]any, <-chan struct{}, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan map[string]any, len(b.history)+256)
	id := b.nextID
	b.nextID++

	// Replay history. Channel is sized to fit all history plus live headroom,
	// so this never blocks while holding the mutex.
	for _, ev := range b.history {
		ch <- ev
	}

	if b.closed {
		close(ch)
		return ch, b.doneCh, func() {}
	}

	b.clients[id] = ch
	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.clients[id]; ok {
			delete(b.clients, id)
			close(ch)
		}
	}
	return ch, b.doneCh, unsub
}

// Close signals that no more events will be sent. All client channels are closed.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	close(b.doneCh)
	for id, ch := range b.clients {
		close(ch)
		delete(b.clients, id)
	}
}

// History returns a copy of all events received so far.
func (b *Broadcaster) History() []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]map[string]any, len(b.history))
	copy(out, b.history)
	return out
}

// WriteSSE streams events from a Broadcaster to an HTTP response as Server-Sent Events.
func WriteSSE(w http.ResponseWriter, r *http.Request, b *Broadcaster) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx proxy compatibility
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, doneCh, unsub := b.Subscribe()
	defer unsub()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				// Channel closed. Only emit "done" if the broadcaster actually
				// finished (vs. this client being dropped for slowness).
				select {
				case <-doneCh:
					fmt.Fprintf(w, "event: done\ndata: {}\n\n")
					flusher.Flush()
				default:
					// Slow-client drop — just disconnect silently.
				}
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
