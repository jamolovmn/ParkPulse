// Package analyzer ANPR va RELAY hodisalarini juftlashtiradi:
//   - juftlik topilsa  -> PassEvent (latency ms bilan)
//   - Relay juftsiz qolsa -> GhostEvent ("arvoh ochilish")
//
// Juftlashtirish birinchi navbatda mashina raqami (plate) bo'yicha — real p24
// loglarida relay/to'lov qatorlarida raqam ham bor. Raqamsiz relay uchun eng
// so'nggi ANPR fallback ishlatiladi.
package analyzer

import (
	"context"
	"os"
	"strconv"
	"time"

	"parkpulse/backend/internal/parser"
)

// Standart qiymatlar env orqali o'zgartiriladi: MATCH_WINDOW_SEC, GRACE_SEC.
// Real loglarda ANPR va to'lov orasida ~86s kuzatildi, shuning uchun oyna keng.
const (
	defaultMatchWindow = 180 * time.Second
	defaultGrace       = 3 * time.Second
)

type PassEvent struct {
	Plate     string    `json:"plate"`
	Gate      string    `json:"gate"`
	AnprAt    time.Time `json:"anpr_at"`
	RelayAt   time.Time `json:"relay_at"`
	LatencyMs float64   `json:"latency_ms"`
}

type GhostEvent struct {
	Plate   string    `json:"plate,omitempty"`
	Gate    string    `json:"gate"`
	RelayAt time.Time `json:"relay_at"`
	Raw     string    `json:"raw"`
}

type Stats struct {
	TotalPasses  int     `json:"total_passes"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	GhostCount   int     `json:"ghost_count"`
}

// Message — WebSocket'ga ketadigan yagona konvert.
type Message struct {
	Type string `json:"type"` // "pass" | "ghost" | "stats"
	Data any    `json:"data"`
}

type anprEntry struct {
	ev     *parser.Event
	seenAt time.Time // wall-clock, eskirganini o'chirish uchun
}

type pendingRelay struct {
	ev         *parser.Event
	receivedAt time.Time
}

type Analyzer struct {
	Out chan Message

	window time.Duration
	grace  time.Duration

	lastANPR map[string]anprEntry // kalit: plate
	pending  []pendingRelay
	// Bitta tranzaksiya bir nechta relay qatori chiqaradi ("already being
	// processed" va h.k.) — bir key uchun oynada faqat bitta natija hisoblanadi.
	lastOutcome map[string]time.Time // kalit: plate (bo'lmasa gate)
	stats       Stats
	sumMs       float64
}

func New() *Analyzer {
	return &Analyzer{
		Out:         make(chan Message, 256),
		window:      envDuration("MATCH_WINDOW_SEC", defaultMatchWindow),
		grace:       envDuration("GRACE_SEC", defaultGrace),
		lastANPR:    make(map[string]anprEntry),
		lastOutcome: make(map[string]time.Time),
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return def
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
			a.expire(now)
		}
	}
}

func (a *Analyzer) handle(ev *parser.Event) {
	switch ev.Type {
	case parser.EventANPR:
		// Kech kelgan ANPR: grace'da turgan Relay bilan juftlanadimi?
		for i, p := range a.pending {
			if plateMatch(ev.Plate, p.ev.Plate) {
				a.pending = append(a.pending[:i], a.pending[i+1:]...)
				a.emitPass(ev, p.ev)
				return
			}
		}
		a.lastANPR[ev.Plate] = anprEntry{ev: ev, seenAt: time.Now()}
	case parser.EventRelay:
		key := outcomeKey(ev)
		if t, ok := a.lastOutcome[key]; ok && time.Since(t) < a.window {
			return // shu tranzaksiya allaqachon hisoblangan — duplikat qator
		}
		if anpr := a.takeANPR(ev); anpr != nil {
			a.emitPass(anpr, ev)
		} else {
			a.pending = append(a.pending, pendingRelay{ev: ev, receivedAt: time.Now()})
		}
	}
}

func outcomeKey(relay *parser.Event) string {
	if relay.Plate != "" {
		return relay.Plate
	}
	return "gate:" + relay.Gate
}

func plateMatch(anprPlate, relayPlate string) bool {
	return relayPlate == "" || anprPlate == relayPlate
}

// takeANPR mos ANPR'ni qidiradi va topilsa o'chiradi (bitta ANPR bitta
// ochilishni "oqlaydi"). Raqamli relay faqat o'z raqamini qabul qiladi.
func (a *Analyzer) takeANPR(relay *parser.Event) *parser.Event {
	if relay.Plate != "" {
		if e, ok := a.lastANPR[relay.Plate]; ok && a.inWindow(e.ev, relay) {
			delete(a.lastANPR, relay.Plate)
			return e.ev
		}
		return nil
	}
	// Raqamsiz relay: oynadagi eng so'nggi ANPR
	var best string
	for plate, e := range a.lastANPR {
		if a.inWindow(e.ev, relay) &&
			(best == "" || e.ev.Timestamp.After(a.lastANPR[best].ev.Timestamp)) {
			best = plate
		}
	}
	if best == "" {
		return nil
	}
	ev := a.lastANPR[best].ev
	delete(a.lastANPR, best)
	return ev
}

func (a *Analyzer) inWindow(anpr, relay *parser.Event) bool {
	d := relay.Timestamp.Sub(anpr.Timestamp)
	return d >= 0 && d <= a.window
}

func (a *Analyzer) expire(now time.Time) {
	kept := a.pending[:0]
	for _, p := range a.pending {
		if now.Sub(p.receivedAt) > a.grace {
			a.stats.GhostCount++
			a.lastOutcome[outcomeKey(p.ev)] = now
			a.Out <- Message{Type: "ghost", Data: GhostEvent{
				Plate: p.ev.Plate, Gate: p.ev.Gate, RelayAt: p.ev.Timestamp, Raw: p.ev.Raw,
			}}
			a.emitStats()
		} else {
			kept = append(kept, p)
		}
	}
	a.pending = kept

	// Eski yozuvlarni tozalash (xotira oqib ketmasin)
	for plate, e := range a.lastANPR {
		if now.Sub(e.seenAt) > a.window+time.Minute {
			delete(a.lastANPR, plate)
		}
	}
	for key, t := range a.lastOutcome {
		if now.Sub(t) > a.window+time.Minute {
			delete(a.lastOutcome, key)
		}
	}
}

func (a *Analyzer) emitPass(anpr, relay *parser.Event) {
	ms := float64(relay.Timestamp.Sub(anpr.Timestamp)) / float64(time.Millisecond)
	a.stats.TotalPasses++
	a.sumMs += ms
	a.stats.AvgLatencyMs = a.sumMs / float64(a.stats.TotalPasses)
	a.lastOutcome[outcomeKey(relay)] = time.Now()
	a.Out <- Message{Type: "pass", Data: PassEvent{
		Plate: anpr.Plate, Gate: relay.Gate,
		AnprAt: anpr.Timestamp, RelayAt: relay.Timestamp, LatencyMs: ms,
	}}
	a.emitStats()
}

func (a *Analyzer) emitStats() {
	a.Out <- Message{Type: "stats", Data: a.stats}
}
