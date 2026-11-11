package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type eventHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

var hub = &eventHub{clients: map[chan string]struct{}{}}

func (h *eventHub) add() chan string {
	ch := make(chan string, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) remove(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *eventHub) broadcast(payload map[string]interface{}) {
	b, _ := json.Marshal(payload)
	msg := "data: " + string(b) + "\n\n"
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *eventHub) sendMsg(text, typ string) {
	if typ == "" {
		typ = "STATUS"
	}
	h.broadcast(map[string]interface{}{"type": typ, "text": text, "time": time.Now().UnixMilli()})
	// keep logs in stderr
	fmt.Printf("[%s] %s\n", typ, text)
}

func (h *eventHub) sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	ch := hub.add()
	defer hub.remove(ch)
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = w.Write([]byte(msg))
			flusher.Flush()
		case <-ticker.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}
