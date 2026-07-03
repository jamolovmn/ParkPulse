// Package analyzer hodisalar zanjirini sessiyalarga yig'adi:
//
//	ANPR (1) -> GATEWAY (2) -> PERMIT/DB (3) -> RELAY/POS (4)
//
//   - zanjir yakunlansa      -> PassEvent (total latency + breakdown)
//   - RELAY juftsiz qolsa    -> GhostEvent ("arvoh ochilish")
//
// Juftlashtirish birinchi navbatda mashina raqami (plate) bo'yicha; raqamsiz
// hodisalar oynadagi eng so'nggi ochiq sessiyaga bog'lanadi.
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

// Breakdown — total latency'ning qadamlararo taqsimoti.
type Breakdown struct {
	GatewayMs float64 `json:"gateway_ms"` // ANPR -> gateway ishga tushdi
	DbMs      float64 `json:"db_ms"`      // gateway -> DB javobi (permit)
	PosMs     float64 `json:"pos_ms"`     // DB -> POS'ga buyruq (relay)
}

type PassEvent struct {
	Plate     string     `json:"plate"`
	Gate      string     `json:"gate"`
	AnprAt    time.Time  `json:"anpr_at"`
	RelayAt   time.Time  `json:"relay_at"`
	LatencyMs float64    `json:"latency_ms"`
	Breakdown *Breakdown `json:"breakdown,omitempty"` // 2-qadam ko'rinmagan bo'lsa yo'q
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

// session — bitta mashinaning ochiq zanjiri (ANPR'dan relay'gacha).
type session struct {
	anpr      *parser.Event
	gatewayAt time.Time // 2-qadam vaqti (zero = hali ko'rinmadi)
	permitAt  time.Time // 3-qadam vaqti
	seenAt    time.Time // wall-clock, eskirganini o'chirish uchun
}

type pendingRelay struct {
	ev         *parser.Event
	receivedAt time.Time
}

type Analyzer struct {
	Out chan Message

	window time.Duration
	grace  time.Duration

	sessions map[string]*session // kalit: plate
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
		sessions:    make(map[string]*session),
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
				a.emitPass(&session{anpr: ev}, p.ev)
				return
			}
		}
		a.sessions[ev.Plate] = &session{anpr: ev, seenAt: time.Now()}

	case parser.EventGateway:
		if s := a.findSession(ev); s != nil && s.gatewayAt.IsZero() {
			s.gatewayAt = ev.Timestamp
		}

	case parser.EventPermit:
		if s := a.findSession(ev); s != nil && s.permitAt.IsZero() {
			s.permitAt = ev.Timestamp
		}

	case parser.EventRelay:
		key := outcomeKey(ev)
		if t, ok := a.lastOutcome[key]; ok && time.Since(t) < a.window {
			return // shu tranzaksiya allaqachon hisoblangan — duplikat qator
		}
		if s := a.takeSession(ev); s != nil {
			a.emitPass(s, ev)
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

// findSession hodisani sessiyaga bog'laydi: raqami bo'lsa — aynan o'sha,
// bo'lmasa oynadagi eng so'nggi ochiq sessiya. Sessiya o'chirilmaydi.
func (a *Analyzer) findSession(ev *parser.Event) *session {
	if ev.Plate != "" {
		if s, ok := a.sessions[ev.Plate]; ok && a.inWindow(s.anpr, ev) {
			return s
		}
		return nil
	}
	if plate := a.latestPlate(ev); plate != "" {
		return a.sessions[plate]
	}
	return nil
}

// takeSession relay uchun sessiyani oladi va o'chiradi (bitta ANPR bitta
// ochilishni "oqlaydi"). Raqamli relay faqat o'z raqamini qabul qiladi.
func (a *Analyzer) takeSession(relay *parser.Event) *session {
	plate := relay.Plate
	if plate == "" {
		plate = a.latestPlate(relay)
	}
	if s, ok := a.sessions[plate]; ok && a.inWindow(s.anpr, relay) {
		delete(a.sessions, plate)
		return s
	}
	return nil
}

// latestPlate oynadagi eng so'nggi ANPR'li sessiya raqamini qaytaradi.
func (a *Analyzer) latestPlate(ev *parser.Event) string {
	var best string
	for plate, s := range a.sessions {
		if a.inWindow(s.anpr, ev) &&
			(best == "" || s.anpr.Timestamp.After(a.sessions[best].anpr.Timestamp)) {
			best = plate
		}
	}
	return best
}

func (a *Analyzer) inWindow(anpr, ev *parser.Event) bool {
	d := ev.Timestamp.Sub(anpr.Timestamp)
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
	for plate, s := range a.sessions {
		if now.Sub(s.seenAt) > a.window+time.Minute {
			delete(a.sessions, plate)
		}
	}
	for key, t := range a.lastOutcome {
		if now.Sub(t) > a.window+time.Minute {
			delete(a.lastOutcome, key)
		}
	}
}

func (a *Analyzer) emitPass(s *session, relay *parser.Event) {
	total := durMs(relay.Timestamp.Sub(s.anpr.Timestamp))
	var br *Breakdown
	if !s.permitAt.IsZero() {
		// Gateway qatori ko'rinmagan bo'lsa DB vaqti to'g'ridan ANPR'dan boshlanadi
		dbFrom := s.anpr.Timestamp
		var gwMs float64
		if !s.gatewayAt.IsZero() {
			gwMs = durMs(s.gatewayAt.Sub(s.anpr.Timestamp))
			dbFrom = s.gatewayAt
		}
		br = &Breakdown{
			GatewayMs: gwMs,
			DbMs:      durMs(s.permitAt.Sub(dbFrom)),
			PosMs:     durMs(relay.Timestamp.Sub(s.permitAt)),
		}
	}
	a.stats.TotalPasses++
	a.sumMs += total
	a.stats.AvgLatencyMs = a.sumMs / float64(a.stats.TotalPasses)
	a.lastOutcome[outcomeKey(relay)] = time.Now()
	a.Out <- Message{Type: "pass", Data: PassEvent{
		Plate: s.anpr.Plate, Gate: relay.Gate,
		AnprAt: s.anpr.Timestamp, RelayAt: relay.Timestamp,
		LatencyMs: total, Breakdown: br,
	}}
	a.emitStats()
}

func durMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func (a *Analyzer) emitStats() {
	a.Out <- Message{Type: "stats", Data: a.stats}
}
