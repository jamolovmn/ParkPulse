// Package ws analyzer xabarlarini barcha ulangan brauzerlarga real vaqtda tarqatadi.
// Yangi ulangan klient avval "snapshot" (KPI + so'nggi hodisalar) oladi.
package ws

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"parkpulse/backend/internal/analyzer"
)

const historyLimit = 50 // snapshot'da saqlanadigan so'nggi hodisalar soni

var upgrader = websocket.Upgrader{
	// Dashboard boshqa port/domenda ishlaydi — origin tekshirmaymiz.
	CheckOrigin: func(*http.Request) bool { return true },
}

type snapshot struct {
	Stats  analyzer.Stats        `json:"stats"`
	Passes []analyzer.PassEvent  `json:"passes"`
	Ghosts []analyzer.GhostEvent `json:"ghosts"`
}

type Hub struct {
	mu      sync.Mutex
	clients map[chan analyzer.Message]struct{}
	state   snapshot
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan analyzer.Message]struct{})}
}

// Run analyzer'dan kelgan xabarlarni tarixga yozadi va klientlarga tarqatadi.
func (h *Hub) Run(ctx context.Context, in <-chan analyzer.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-in:
			h.mu.Lock()
			h.remember(msg)
			for ch := range h.clients {
				select {
				case ch <- msg:
				default: // sekin klient hammani to'sib qo'ymasin — xabar tashlab ketiladi
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) remember(msg analyzer.Message) {
	switch d := msg.Data.(type) {
	case analyzer.Stats:
		h.state.Stats = d
	case analyzer.PassEvent:
		h.state.Passes = appendCapped(h.state.Passes, d)
	case analyzer.GhostEvent:
		h.state.Ghosts = appendCapped(h.state.Ghosts, d)
	}
}

func appendCapped[T any](s []T, v T) []T {
	s = append(s, v)
	if len(s) > historyLimit {
		s = s[len(s)-historyLimit:]
	}
	return s
}

// HandleWS — GET /ws endpointi.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch := make(chan analyzer.Message, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	snap := h.state
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	if err := conn.WriteJSON(analyzer.Message{Type: "snapshot", Data: snap}); err != nil {
		return
	}

	// O'qish goroutine'i faqat klient uzilganini sezish uchun.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	log.Printf("[ws] klient ulandi: %s", r.RemoteAddr)
	for {
		select {
		case <-done:
			return
		case msg := <-ch:
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}
