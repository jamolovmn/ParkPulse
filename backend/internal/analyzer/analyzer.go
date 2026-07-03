package analyzer

import (
	"context"
	"time"

	"parkpulse/backend/internal/parser"
)

const (
	MatchWindow = 5 * time.Second
	GracePeriod = 2 * time.Second
)

type PassEvent struct {
	Plate     string    `json:"plate"`
	Gate      string    `json:"gate"`
	AnprAt    time.Time `json:"anpr_at"`
	RelayAt   time.Time `json:"relay_at"`
	LatencyMs float64   `json:"latency_ms"`
}

type GhostEvent struct {
	Gate    string    `json:"gate"`
	RelayAt time.Time `json:"relay_at"`
	Raw     string    `json:"raw"`
}

type Stats struct {
	TotalPasses  int     `json:"total_passes"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	GhostCount   int     `json:"ghost_count"`
}

type Message struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type pendingRelay struct {
	ev         *parser.Event
	receivedAt time.Time
}

type Analyzer struct {
	Out chan Message

	lastANPR map[string]*parser.Event
	pending  []pendingRelay
	stats    Stats
	sumMs    float64
}

func New() *Analyzer {
	return &Analyzer{
		Out:      make(chan Message, 256),
		lastANPR: make(map[string]*parser.Event),
	}
}

func (a *Analyzer) Run(ctx context.Context, in <-chan *parser.Event) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-in:
			a.handle(ev)
		case now := <-tick.C:
			a.expirePending(now)
		}
	}
}

func (a *Analyzer) handle(ev *parser.Event) {
	switch ev.Type {
	case parser.EventANPR:
		// Kech kelgan ANPR: grace'da turgan Relay bilan juftlanadimi?
		for i, p := range a.pending {
			if gateMatch(ev.Gate, p.ev.Gate) {
				a.pending = append(a.pending[:i], a.pending[i+1:]...)
				a.emitPass(ev, p.ev)
				return
			}
		}
		a.lastANPR[ev.Gate] = ev
	case parser.EventRelay:
		if anpr := a.takeANPR(ev); anpr != nil {
			a.emitPass(anpr, ev)
		} else {
			a.pending = append(a.pending, pendingRelay{ev: ev, receivedAt: time.Now()})
		}
	}
}

// takeANPR Relay uchun mos ANPR'ni qidiradi va topilsa ro'yxatdan o'chiradi
// (bitta ANPR ikkita ochilishni "oqlab" yubormasligi uchun).
func (a *Analyzer) takeANPR(relay *parser.Event) *parser.Event {
	// 1) Aynan shu darvoza; 2) darvozasi noma'lum ANPR (fallback).
	for _, key := range []string{relay.Gate, ""} {
		if anpr, ok := a.lastANPR[key]; ok && inWindow(anpr, relay) {
			delete(a.lastANPR, key)
			return anpr
		}
	}
	return nil
}

func inWindow(anpr, relay *parser.Event) bool {
	d := relay.Timestamp.Sub(anpr.Timestamp)
	return d >= 0 && d <= MatchWindow
}

func gateMatch(a, b string) bool {
	return a == b || a == "" || b == ""
}

func (a *Analyzer) expirePending(now time.Time) {
	kept := a.pending[:0]
	for _, p := range a.pending {
		if now.Sub(p.receivedAt) > GracePeriod {
			a.stats.GhostCount++
			a.Out <- Message{Type: "ghost", Data: GhostEvent{
				Gate: p.ev.Gate, RelayAt: p.ev.Timestamp, Raw: p.ev.Raw,
			}}
			a.emitStats()
		} else {
			kept = append(kept, p)
		}
	}
	a.pending = kept
}

func (a *Analyzer) emitPass(anpr, relay *parser.Event) {
	ms := float64(relay.Timestamp.Sub(anpr.Timestamp)) / float64(time.Millisecond)
	a.stats.TotalPasses++
	a.sumMs += ms
	a.stats.AvgLatencyMs = a.sumMs / float64(a.stats.TotalPasses)
	a.Out <- Message{Type: "pass", Data: PassEvent{
		Plate: anpr.Plate, Gate: relay.Gate,
		AnprAt: anpr.Timestamp, RelayAt: relay.Timestamp, LatencyMs: ms,
	}}
	a.emitStats()
}

func (a *Analyzer) emitStats() {
	a.Out <- Message{Type: "stats", Data: a.stats}
}
