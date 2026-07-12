// Package ws analyzer xabarlarini barcha ulangan brauzerlarga real vaqtda tarqatadi.
// Yangi ulangan klient avval "snapshot" (KPI + so'nggi hodisalar) oladi.
package ws

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"parkpulse/backend/internal/analyzer"
	"parkpulse/backend/internal/collector"
	"parkpulse/backend/internal/netmon"
	"parkpulse/backend/internal/snmp"
	"parkpulse/backend/internal/speedtest"
)

const (
	historyLimit = 50  // snapshot'da saqlanadigan so'nggi hodisalar soni
	ghostLimit   = 200 // shubhali ochilishlar (arvoh + qoidabuzarlik) alohida, uzoq saqlanadi

	// Oradagi proxy'lar (ioEdge va h.k.) jim turgan ulanishni ~60s da uzadi.
	// Server har 30s da ping yuboradi — ulanish doim "tirik" qoladi.
	pingInterval = 30 * time.Second
	pongWait     = 75 * time.Second
	writeWait    = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	// Dashboard boshqa port/domenda ishlaydi — origin tekshirmaymiz.
	CheckOrigin: func(*http.Request) bool { return true },
}

type snapshot struct {
	Stats  analyzer.Stats       `json:"stats"`
	Passes []analyzer.PassEvent `json:"passes"`
	Opens  []analyzer.OpenEvent `json:"opens"` // so'nggi ochilishlar (barcha turlar, 50 ta)
	// Ghosts — faqat shubhali ochilishlar (arvoh + qoidabuzarlik). Alohida va
	// uzoq saqlanadi: band darvozada oddiy ochilishlar 50 talik oynadan bittagina
	// arvohni siqib chiqarib yubormasin — KPI hisoblagichi bilan mos qolsin.
	Ghosts  []analyzer.OpenEvent    `json:"ghosts"`
	Traffic []analyzer.TrafficPoint `json:"traffic"`
	Devices []netmon.Device         `json:"devices"`
	SNMP    []snmp.Host             `json:"snmp"`
	Speed   *speedtest.Result       `json:"speed,omitempty"`
	Health  *collector.Health       `json:"health"`
}

type Hub struct {
	mu      sync.Mutex
	clients map[chan analyzer.Message]struct{}
	state   snapshot
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan analyzer.Message]struct{}),
		// Birinchi snapshot'da ham JSON shakli to'liq bo'lsin (null emas).
		state: snapshot{Stats: analyzer.Stats{Opens: map[string]int{}}},
	}
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
		h.state.Passes = appendCapped(h.state.Passes, d, historyLimit)
	case analyzer.OpenEvent:
		h.state.Opens = appendCapped(h.state.Opens, d, historyLimit)
		if d.Kind.Suspicious() {
			h.state.Ghosts = appendCapped(h.state.Ghosts, d, ghostLimit)
		}
	case []analyzer.TrafficPoint:
		h.state.Traffic = d
	case []netmon.Device:
		h.state.Devices = d
	case []snmp.Host:
		h.state.SNMP = d
	case *speedtest.Result:
		h.state.Speed = d
	case collector.Health:
		h.state.Health = &d
	}
}

func appendCapped[T any](s []T, v T, limit int) []T {
	s = append(s, v)
	if len(s) > limit {
		s = s[len(s)-limit:]
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

	// O'qish goroutine'i klient uzilganini sezadi va pong'larni qabul qiladi.
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
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
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case msg := <-ch:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}
