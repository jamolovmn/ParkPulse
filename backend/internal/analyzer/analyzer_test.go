package analyzer

import (
	"testing"
	"time"

	"parkpulse/backend/internal/parser"
)

var t0 = time.Date(2026, 7, 3, 12, 59, 2, 65187000, time.UTC)

func ev(typ parser.EventType, plate, gate string, at time.Time) *parser.Event {
	return &parser.Event{Type: typ, Plate: plate, Gate: gate, Timestamp: at, Raw: "test"}
}

// drain Out kanalidan hamma xabarni oladi (bufer 256, test uchun yetarli).
func drain(a *Analyzer) []Message {
	var out []Message
	for {
		select {
		case m := <-a.Out:
			out = append(out, m)
		default:
			return out
		}
	}
}

func findPass(t *testing.T, msgs []Message) PassEvent {
	t.Helper()
	for _, m := range msgs {
		if m.Type == "pass" {
			return m.Data.(PassEvent)
		}
	}
	t.Fatal("pass topilmadi")
	return PassEvent{}
}

// To'liq zanjir (real vaqtlar bilan): ANPR -> gateway(+0.2ms) -> permit(+12ms) -> relay(+0.8ms).
func TestChainBreakdown(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "60X339HB", "", t0))
	a.handle(ev(parser.EventGateway, "", "", t0.Add(200*time.Microsecond)))
	a.handle(ev(parser.EventPermit, "", "", t0.Add(12200*time.Microsecond)))
	a.handle(ev(parser.EventRelay, "60X339HB", "exit 1", t0.Add(13000*time.Microsecond)))

	p := findPass(t, drain(a))
	if p.LatencyMs != 13 {
		t.Errorf("total = %v, kutilgan 13", p.LatencyMs)
	}
	if p.Breakdown == nil {
		t.Fatal("breakdown yo'q")
	}
	if p.Breakdown.GatewayMs != 0.2 || p.Breakdown.DbMs != 12 || p.Breakdown.PosMs != 0.8 {
		t.Errorf("breakdown = %+v, kutilgan gateway=0.2 db=12 pos=0.8", p.Breakdown)
	}
}

// Gateway qatori bo'lmasa DB vaqti ANPR'dan hisoblanadi.
func TestChainNoGateway(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "60X339HB", "", t0))
	a.handle(ev(parser.EventPermit, "", "", t0.Add(12*time.Millisecond)))
	a.handle(ev(parser.EventRelay, "60X339HB", "exit 1", t0.Add(13*time.Millisecond)))

	p := findPass(t, drain(a))
	if p.Breakdown == nil || p.Breakdown.GatewayMs != 0 || p.Breakdown.DbMs != 12 {
		t.Errorf("breakdown = %+v, kutilgan gateway=0 db=12", p.Breakdown)
	}
}

// Permit ko'rinmasa breakdown bo'lmaydi, lekin pass baribir chiqadi.
func TestNoPermitNoBreakdown(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventRelay, "01M635ZB", "exit 1", t0.Add(400*time.Millisecond)))

	p := findPass(t, drain(a))
	if p.Breakdown != nil {
		t.Errorf("breakdown kutilmagan edi: %+v", p.Breakdown)
	}
	if p.LatencyMs != 400 {
		t.Errorf("total = %v, kutilgan 400", p.LatencyMs)
	}
}

// Ghost mantiqi buzilmaganini tekshirish: ANPR ko'rmagan raqam -> ghost.
func TestGhostStillDetected(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventRelay, "77X777XX", "exit 1", t0.Add(time.Second)))
	a.expire(time.Now().Add(defaultGrace + time.Second))

	var ghost *GhostEvent
	for _, m := range drain(a) {
		if m.Type == "ghost" {
			g := m.Data.(GhostEvent)
			ghost = &g
		}
	}
	if ghost == nil || ghost.Plate != "77X777XX" {
		t.Fatalf("ghost topilmadi yoki xato: %+v", ghost)
	}
}

// Duplikat relay qatorlari bitta natija bo'lib qolishi kerak.
func TestDuplicateRelaySuppressed(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventRelay, "01M635ZB", "exit 1", t0.Add(time.Second)))
	a.handle(ev(parser.EventRelay, "01M635ZB", "exit 1", t0.Add(2*time.Second))) // duplikat
	a.expire(time.Now().Add(defaultGrace + time.Second))

	passes, ghosts := 0, 0
	for _, m := range drain(a) {
		switch m.Type {
		case "pass":
			passes++
		case "ghost":
			ghosts++
		}
	}
	if passes != 1 || ghosts != 0 {
		t.Errorf("passes=%d ghosts=%d, kutilgan 1/0", passes, ghosts)
	}
}
