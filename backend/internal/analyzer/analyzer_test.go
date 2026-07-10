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

func opens(msgs []Message) []OpenEvent {
	var out []OpenEvent
	for _, m := range msgs {
		if m.Type == "open" {
			out = append(out, m.Data.(OpenEvent))
		}
	}
	return out
}

// onlyOpen bitta ochilish chiqqanini va turini tekshiradi.
func onlyOpen(t *testing.T, a *Analyzer, want OpenKind) OpenEvent {
	t.Helper()
	os := opens(drain(a))
	if len(os) != 1 {
		t.Fatalf("ochilishlar soni = %d, kutilgan 1: %+v", len(os), os)
	}
	if os[0].Kind != want {
		t.Fatalf("kind = %q, kutilgan %q", os[0].Kind, want)
	}
	return os[0]
}

// To'liq zanjir (real vaqtlar bilan): ANPR -> gateway(+0.2ms) -> permit(+12ms) -> POS(+0.8ms).
func TestChainBreakdown(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "60X339HB", "", t0))
	a.handle(ev(parser.EventGateway, "", "", t0.Add(200*time.Microsecond)))
	a.handle(ev(parser.EventPermit, "", "", t0.Add(12200*time.Microsecond)))
	a.handle(ev(parser.EventPOS, "60X339HB", "exit 1", t0.Add(13000*time.Microsecond)))

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
	a.handle(ev(parser.EventPOS, "60X339HB", "exit 1", t0.Add(13*time.Millisecond)))

	p := findPass(t, drain(a))
	if p.Breakdown == nil || p.Breakdown.GatewayMs != 0 || p.Breakdown.DbMs != 12 {
		t.Errorf("breakdown = %+v, kutilgan gateway=0 db=12", p.Breakdown)
	}
}

// Permit ko'rinmasa breakdown bo'lmaydi, lekin pass baribir chiqadi.
func TestNoPermitNoBreakdown(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(400*time.Millisecond)))

	p := findPass(t, drain(a))
	if p.Breakdown != nil {
		t.Errorf("breakdown kutilmagan edi: %+v", p.Breakdown)
	}
	if p.LatencyMs != 400 {
		t.Errorf("total = %v, kutilgan 400", p.LatencyMs)
	}
}

// Holat 1: to'lov qilindi -> shlagbaum ochildi. Apparat qatori duplikat sanaladi.
func TestKindPaid(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPermit, "01M635ZB", "", t0.Add(10*time.Millisecond)))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(time.Second)))
	a.handle(ev(parser.EventOpen, "", "exit 1", t0.Add(2*time.Second))) // apparat
	a.expire(time.Now().Add(2 * defaultAutoPay))

	o := onlyOpen(t, a, KindPaid)
	if o.Plate != "01M635ZB" || o.Context != nil {
		t.Errorf("paid uchun log yozilmasligi kerak: %+v", o)
	}
	if a.stats.GhostCount != 0 {
		t.Errorf("ghost_count = %d, kutilgan 0", a.stats.GhostCount)
	}
}

// Holat 2: mashina datchikda, pult bilan ochildi, chiqqach avto to'lov olindi.
func TestKindRemoteByAutoPay(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPermit, "01M635ZB", "", t0.Add(10*time.Millisecond)))
	a.handle(ev(parser.EventOpen, "", "exit 1", t0.Add(2*time.Second))) // pult bosildi
	if len(a.pending) != 1 {
		t.Fatalf("ochilish avto-to'lovni kutishi kerak edi: %+v", a.pending)
	}
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(20*time.Second))) // avto to'lov

	o := onlyOpen(t, a, KindRemote)
	if o.Context != nil || o.Raw != "" {
		t.Errorf("remote uchun log yozilmasligi kerak: %+v", o)
	}
	if a.stats.GhostCount != 0 {
		t.Errorf("ghost_count = %d, kutilgan 0", a.stats.GhostCount)
	}
}

// Pult bilan ochilgan mashinaning "latency"'si — haydovchining turish vaqti.
// U o'rtachani buzmasligi kerak (aks holda KPI soniyalarga siljiydi).
func TestAutoPayExcludedFromAvgLatency(t *testing.T) {
	a := New()
	// Oddiy o'tish: 400 ms
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(400*time.Millisecond)))
	// Pult bilan: ochilish avval, to'lov 25 soniyadan keyin
	a.handle(ev(parser.EventANPR, "60X339HB", "", t0.Add(time.Minute)))
	a.handle(ev(parser.EventOpen, "", "exit 1", t0.Add(65*time.Second)))
	a.handle(ev(parser.EventPOS, "60X339HB", "exit 1", t0.Add(85*time.Second)))

	var auto, normal PassEvent
	for _, m := range drain(a) {
		if m.Type != "pass" {
			continue
		}
		if p := m.Data.(PassEvent); p.AutoPay {
			auto = p
		} else {
			normal = p
		}
	}
	if !auto.AutoPay || auto.Plate != "60X339HB" {
		t.Fatalf("avto to'lov belgilanmadi: %+v", auto)
	}
	if normal.AutoPay {
		t.Fatalf("oddiy o'tish avto deb belgilandi: %+v", normal)
	}
	if a.stats.TotalPasses != 2 {
		t.Errorf("total_passes = %d, kutilgan 2", a.stats.TotalPasses)
	}
	if a.stats.AvgLatencyMs != 400 {
		t.Errorf("avg = %v ms, kutilgan 400 (25s avto to'lov qo'shilmasin)", a.stats.AvgLatencyMs)
	}
}

// Holat 2': pult signali logda bo'lsa, avto-to'lovni kutmasdan darhol aniqlanadi.
func TestKindRemoteBySignal(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventRemote, "", "exit 1", t0.Add(2*time.Second)))
	a.handle(ev(parser.EventOpen, "", "exit 1", t0.Add(3*time.Second)))

	onlyOpen(t, a, KindRemote)
	if len(a.pending) != 0 {
		t.Errorf("pult signali bilan kutish shart emas: %+v", a.pending)
	}
}

// Holat 3: mashina datchikda, qarzi bor, pult ham bosilmadi — lekin ochildi.
func TestKindViolation(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPermit, "01M635ZB", "", t0.Add(10*time.Millisecond)))
	a.handle(ev(parser.EventOpen, "", "exit 1", t0.Add(2*time.Second)))
	a.expire(time.Now().Add(defaultAutoPay + time.Second)) // to'lov kelmadi

	o := onlyOpen(t, a, KindViolation)
	if o.Plate != "01M635ZB" {
		t.Errorf("plate = %q, kutilgan 01M635ZB", o.Plate)
	}
	if a.stats.GhostCount != 1 {
		t.Errorf("ghost_count = %d, kutilgan 1", a.stats.GhostCount)
	}
}

// Kirishda qarzdor mashina ham o'tadi va to'lov so'ralmaydi — qoidabuzarlik EMAS.
func TestEnterDebtorIsNotViolation(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPermit, "01M635ZB", "", t0.Add(10*time.Millisecond))) // qarzi bor
	a.handle(ev(parser.EventOpen, "", "enter 1", t0.Add(2*time.Second)))
	a.expire(time.Now().Add(defaultAutoPay + time.Second)) // to'lov kelmaydi — normal

	o := onlyOpen(t, a, KindEntry)
	if o.Context != nil || o.Raw != "" {
		t.Errorf("kirish uchun log yozilmasligi kerak: %+v", o)
	}
	if a.stats.GhostCount != 0 {
		t.Errorf("ghost_count = %d, kutilgan 0 (kirish arvoh emas)", a.stats.GhostCount)
	}
	if len(a.pending) != 0 {
		t.Errorf("kirish avto-to'lovni kutmasligi kerak: %+v", a.pending)
	}
}

// Bitta mashina kirib, oynadan tez chiqsa — ikkala ochilish ham ko'rinadi.
func TestEnterThenExitSamePlate(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventOpen, "", "enter 1", t0.Add(time.Second)))
	// 20 soniyadan keyin chiqadi va to'laydi — dedupe (60s) ichida, lekin darvoza boshqa
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0.Add(20*time.Second)))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(21*time.Second)))

	kinds := map[OpenKind]int{}
	for _, o := range opens(drain(a)) {
		kinds[o.Kind]++
	}
	if kinds[KindEntry] != 1 || kinds[KindPaid] != 1 {
		t.Fatalf("kutilgan entry=1 paid=1, olindi %v", kinds)
	}
}

// Holat 4: datchikda mashina yo'q — o'zidan-o'zi ochildi.
func TestKindGhost(t *testing.T) {
	a := New()
	a.ContextFn = func(string) []string { return []string{"log qatori"} }
	a.handle(ev(parser.EventOpen, "", "exit 1", t0))
	a.expire(time.Now().Add(defaultGrace + time.Second))

	o := onlyOpen(t, a, KindGhost)
	if len(o.Context) != 1 {
		t.Errorf("arvoh uchun log kontekst yozilishi kerak: %+v", o.Context)
	}
	if a.stats.GhostCount != 1 {
		t.Errorf("ghost_count = %d, kutilgan 1", a.stats.GhostCount)
	}
}

// ANPR ko'rmagan raqamga to'lov so'ralsa — bu ham arvoh.
func TestGhostFromUnknownPlate(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPOS, "77X777XX", "exit 1", t0.Add(time.Second)))
	a.expire(time.Now().Add(defaultGrace + time.Second))

	o := onlyOpen(t, a, KindGhost)
	if o.Plate != "77X777XX" {
		t.Fatalf("plate = %q, kutilgan 77X777XX", o.Plate)
	}
}

// Kech kelgan ANPR: ochilish avval loglandi, raqam keyin — arvoh emas.
func TestLateAnprIsNotGhost(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventOpen, "", "exit 1", t0))
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0.Add(-500*time.Millisecond)))
	a.expire(time.Now().Add(defaultGrace + time.Second))

	if got := opens(drain(a)); len(got) != 0 {
		t.Fatalf("hali qaror qabul qilinmasligi kerak edi: %+v", got)
	}
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(10*time.Second)))
	onlyOpen(t, a, KindRemote)
}

// Duplikat qatorlar bitta natija bo'lib qolishi kerak.
func TestDuplicateSuppressed(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(time.Second)))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(2*time.Second))) // duplikat
	a.expire(time.Now().Add(defaultGrace + time.Second))

	msgs := drain(a)
	passes := 0
	for _, m := range msgs {
		if m.Type == "pass" {
			passes++
		}
	}
	if got := opens(msgs); passes != 1 || len(got) != 1 {
		t.Errorf("passes=%d opens=%d, kutilgan 1/1", passes, len(got))
	}
}

// Ketma-ket ikki mashina bitta darvozadan o'tsa, ikkalasi ham hisoblanadi.
func TestTwoCarsSameGate(t *testing.T) {
	a := New()
	a.handle(ev(parser.EventANPR, "01M635ZB", "", t0))
	a.handle(ev(parser.EventPOS, "01M635ZB", "exit 1", t0.Add(time.Second)))
	a.handle(ev(parser.EventANPR, "60X339HB", "", t0.Add(30*time.Second)))
	a.handle(ev(parser.EventPOS, "60X339HB", "exit 1", t0.Add(31*time.Second)))

	if got := opens(drain(a)); len(got) != 2 {
		t.Fatalf("ochilishlar soni = %d, kutilgan 2: %+v", len(got), got)
	}
}

// 24 soatlik grafik: kirish/chiqish alohida sanaladi, arvoh sanalmaydi.
func TestTrafficCounts(t *testing.T) {
	a := New()
	now := time.Now().UTC()
	a.addTraffic(now, "enter 1")
	a.addTraffic(now, "exit 1")
	a.addTraffic(now, "exit 2")

	series := a.Traffic()
	if len(series) != trafficHours {
		t.Fatalf("nuqtalar soni = %d, kutilgan %d", len(series), trafficHours)
	}
	last := series[len(series)-1]
	if last.Enter != 1 || last.Exit != 2 {
		t.Errorf("oxirgi soat = %+v, kutilgan enter=1 exit=2", last)
	}
}
