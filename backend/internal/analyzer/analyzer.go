// Package analyzer hodisalar zanjirini sessiyalarga yig'adi:
//
//		ANPR (1) -> GATEWAY (2) -> PERMIT/DB (3) -> POS (4)
//
//	  - zanjir yakunlansa -> PassEvent (total latency + breakdown)
//	  - shlagbaum ochilsa -> OpenEvent, 4 turdan biri bilan:
//
//	    paid      (Holat 1) — dasturda to'lov qilindi, keyin ochildi.
//	    remote    (Holat 2) — mashina datchikda, pult bilan ochildi;
//	    chiqib ketgach tizim avtomatik pul yechdi.
//	    violation (Holat 3) — mashina datchikda, qarzi bor, to'lov ham,
//	    pult ham yo'q — lekin ochildi.
//	    ghost     (Holat 4) — datchikda mashina umuman yo'q, o'zidan-o'zi ochildi.
//
// Faqat violation va ghost — haqiqiy "arvoh ochilish": ular hisoblanadi va
// log konteksti bilan saqlanadi. paid/remote muammosiz, log yozilmaydi.
//
// Holat 2 va 3 ni ajratish: pult signali logda bo'lsa — o'shanga qaraymiz;
// bo'lmasa ochilishdan keyin AUTOPAY_SEC ichida to'lov kelishini kutamiz.
// To'lov keldi -> pult (Holat 2), kelmadi -> qoidabuzarlik (Holat 3).
package analyzer

import (
	"os"
	"strconv"
	"strings"
	"time"

	"context"

	"parkpulse/backend/internal/parser"
)

// Standart qiymatlar env orqali o'zgartiriladi.
// Real loglarda ANPR va to'lov orasida ~86s kuzatildi, shuning uchun oyna keng.
const (
	defaultMatchWindow = 180 * time.Second // ANPR -> POS zanjiri uchun
	defaultGrace       = 3 * time.Second   // kech kelgan ANPR'ni kutish
	defaultAutoPay     = 90 * time.Second  // ochilishdan keyin avto-to'lovni kutish
	defaultPresence    = 60 * time.Second  // ANPR shuncha vaqt "mashina datchikda"
	defaultDedupe      = 60 * time.Second  // bitta raqamning takroriy qatorlari
	// Raqamsiz qatorlar (RelayWorker) faqat darvoza bo'yicha juftlanadi. Shlagbaum
	// ochilib-yopilishi ~10s, shuning uchun bundan qisqa oraliq — bitta ochilish.
	defaultGateDedupe = 10 * time.Second
	trafficHours      = 24 // grafik uchun soatlik oyna
)

// OpenKind — shlagbaum ochilishining turlari.
type OpenKind string

const (
	KindPaid      OpenKind = "paid"      // Holat 1
	KindRemote    OpenKind = "remote"    // Holat 2
	KindViolation OpenKind = "violation" // Holat 3
	KindGhost     OpenKind = "ghost"     // Holat 4
	KindEntry     OpenKind = "entry"     // kirish — to'lovsiz, muammosiz
)

// Suspicious — faqat shu ikkisi "arvoh ochilish" deb hisoblanadi va loglanadi.
func (k OpenKind) Suspicious() bool { return k == KindViolation || k == KindGhost }

func reasonOf(k OpenKind) string {
	switch k {
	case KindPaid:
		return "Dasturda to'lov qilindi — shlagbaum ochildi"
	case KindRemote:
		return "Pult bilan ochildi — chiqishda avto to'lov olindi"
	case KindEntry:
		return "Mashina kirdi — kirishda to'lov olinmaydi"
	case KindViolation:
		return "Mashina datchikda, qarzi bor — to'lovsiz va pultsiz ochildi"
	default:
		return "Datchikda mashina yo'q — shlagbaum o'zidan-o'zi ochildi"
	}
}

// isEnter — kirish darvozasi. Kirishda qarzdor ham o'tadi va to'lov so'ralmaydi,
// shuning uchun "to'lov kelmadi -> qoidabuzarlik" qoidasi bu yerda ishlamaydi.
func isEnter(gate string) bool { return strings.HasPrefix(gate, "enter") }

// Breakdown — total latency'ning qadamlararo taqsimoti.
type Breakdown struct {
	GatewayMs float64 `json:"gateway_ms"` // ANPR -> gateway ishga tushdi
	DbMs      float64 `json:"db_ms"`      // gateway -> DB javobi (permit)
	PosMs     float64 `json:"pos_ms"`     // DB -> POS'ga to'lov so'rovi
}

type PassEvent struct {
	Plate     string     `json:"plate"`
	Gate      string     `json:"gate"`
	AnprAt    time.Time  `json:"anpr_at"`
	RelayAt   time.Time  `json:"relay_at"`
	LatencyMs float64    `json:"latency_ms"`
	Breakdown *Breakdown `json:"breakdown,omitempty"` // 2-qadam ko'rinmagan bo'lsa yo'q
	// AutoPay — to'lov shlagbaum ochilgandan KEYIN olingan (Holat 2). Bunda
	// latency tizim tezligi emas, haydovchining turish vaqti — o'rtachaga
	// qo'shilmaydi, aks holda KPI o'nlab soniyaga siljib ketadi.
	AutoPay bool `json:"auto_pay,omitempty"`
}

// OpenEvent — shlagbaumning har bir ochilishi, turi bilan.
type OpenEvent struct {
	Kind   OpenKind  `json:"kind"`
	Plate  string    `json:"plate,omitempty"`
	Gate   string    `json:"gate"`
	OpenAt time.Time `json:"open_at"`
	Reason string    `json:"reason"`
	// Raw va Context faqat Suspicious() turlar uchun to'ldiriladi. Muammosiz
	// ochilishlar (paid/remote) ro'yxatda ko'rinadi, lekin log yozilmaydi.
	Raw     string   `json:"raw,omitempty"`
	Context []string `json:"context,omitempty"`
}

type Stats struct {
	TotalPasses  int            `json:"total_passes"`
	AvgLatencyMs float64        `json:"avg_latency_ms"`
	GhostCount   int            `json:"ghost_count"` // faqat violation + ghost
	Opens        map[string]int `json:"opens"`       // OpenKind -> soni
}

// TrafficPoint — bir soatdagi kirish/chiqish soni (24 soatlik grafik uchun).
type TrafficPoint struct {
	Hour  time.Time `json:"hour"`
	Enter int       `json:"enter"`
	Exit  int       `json:"exit"`
}

// Message — WebSocket'ga ketadigan yagona konvert.
type Message struct {
	Type string `json:"type"` // "pass" | "open" | "stats" | "traffic"
	Data any    `json:"data"`
}

// session — bitta mashinaning ochiq zanjiri (ANPR'dan ochilishgacha).
type session struct {
	anpr      *parser.Event
	gatewayAt time.Time // 2-qadam vaqti (zero = hali ko'rinmadi)
	permitAt  time.Time // 3-qadam vaqti (permit bor = tizimda qarz yozilgan)
	posAt     time.Time // to'lov so'rovi vaqti (zero = to'lanmagan)
	openAt    time.Time // shlagbaum ochilgan vaqt (zero = mashina hali datchikda)
	passSent  bool
	seenAt    time.Time // wall-clock, eskirganini o'chirish uchun
}

// pendingOpen — turi hali aniqlanmagan ochilish; deadline'da hal qilinadi.
type pendingOpen struct {
	ev       *parser.Event
	sess     *session // nil = datchikda mashina yo'q
	deadline time.Time
	context  []string
}

type Analyzer struct {
	Out chan Message

	// ContextFn arvoh paytidagi atrofdagi loglarni beradi (main'da ulanadi).
	ContextFn func(container string) []string

	window     time.Duration
	grace      time.Duration
	autoPay    time.Duration
	presence   time.Duration
	dedupe     time.Duration
	gateDedupe time.Duration

	sessions map[string]*session // kalit: plate
	pending  []pendingOpen
	remoteAt map[string]time.Time // kalit: gate — oxirgi pult bosilishi
	// Bitta tranzaksiya bir nechta qator chiqaradi (POS so'rovi + apparat
	// ochilishi + "already being processed") — bir key uchun bitta natija.
	// Kalit: plate yoki "gate:exit 1", qiymat: hodisaning LOG vaqti.
	lastOutcome map[string]time.Time
	// clock — ko'rilgan eng so'nggi log vaqti. Duplikatlar log vaqti bo'yicha
	// solishtiriladi: wall-clock bilan aralashtirilsa, bir darvozadan ketma-ket
	// o'tgan ikki mashina bitta ochilish bo'lib qolardi.
	clock    time.Time
	stats    Stats
	sumMs    float64
	latencyN int                     // o'rtachaga kirgan o'tishlar (AutoPay'lar sanalmaydi)
	traffic  map[int64]*TrafficPoint // kalit: soat boshining unix vaqti
}

func New() *Analyzer {
	return &Analyzer{
		Out:         make(chan Message, 256),
		window:      envDuration("MATCH_WINDOW_SEC", defaultMatchWindow),
		grace:       envDuration("GRACE_SEC", defaultGrace),
		autoPay:     envDuration("AUTOPAY_SEC", defaultAutoPay),
		presence:    envDuration("PRESENCE_SEC", defaultPresence),
		dedupe:      envDuration("DEDUPE_SEC", defaultDedupe),
		gateDedupe:  envDuration("GATE_DEDUPE_SEC", defaultGateDedupe),
		sessions:    make(map[string]*session),
		remoteAt:    make(map[string]time.Time),
		lastOutcome: make(map[string]time.Time),
		traffic:     make(map[int64]*TrafficPoint),
		stats:       Stats{Opens: make(map[string]int)},
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
	// Soat o'tgani sayin 24 soatlik oyna suriladi — hodisa bo'lmasa ham.
	slide := time.NewTicker(time.Hour)
	defer slide.Stop()

	a.emitTraffic() // grafik bo'sh o'q bilan darhol chizilsin
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-in:
			a.handle(ev)
		case now := <-tick.C:
			a.expire(now)
		case <-slide.C:
			a.emitTraffic()
		}
	}
}

func (a *Analyzer) handle(ev *parser.Event) {
	if ev.Timestamp.After(a.clock) {
		a.clock = ev.Timestamp
	}
	switch ev.Type {
	case parser.EventANPR:
		s := &session{anpr: ev, seenAt: time.Now()}
		a.sessions[ev.Plate] = s
		a.adoptPending(s) // ochilish avval, ANPR kechikib kelgan bo'lishi mumkin

	case parser.EventGateway:
		if s := a.sessionFor(ev); s != nil && s.gatewayAt.IsZero() {
			s.gatewayAt = ev.Timestamp
		}

	case parser.EventPermit:
		if s := a.sessionFor(ev); s != nil && s.permitAt.IsZero() {
			s.permitAt = ev.Timestamp
		}

	case parser.EventRemote:
		a.remoteAt[ev.Gate] = ev.Timestamp

	case parser.EventPOS:
		s := a.sessionFor(ev)
		if s != nil && s.posAt.IsZero() {
			s.posAt = ev.Timestamp
			if !s.passSent {
				s.passSent = true
				a.emitPass(s, ev)
			}
		}
		// Holat 2: ochilish avval bo'lgan, endi avto-to'lov keldi.
		if a.resolveByPayment(s) {
			return
		}
		// To'lov so'rovining o'zi ham ochilish qarori — Holat 1 shu yerda tug'iladi.
		a.registerOpen(ev, s)

	case parser.EventOpen:
		a.registerOpen(ev, a.sessionFor(ev))
	}
}

// registerOpen ochilishni darhol tasniflaydi yoki pending'ga qo'yadi.
func (a *Analyzer) registerOpen(ev *parser.Event, s *session) {
	if a.suppressed(ev) || a.pendingDup(ev) {
		return // shu ochilish allaqachon hisoblangan yoki hisoblanmoqda
	}

	if s == nil {
		// Datchikda mashina yo'q. ANPR kechikib kelishi mumkin — grace kutamiz.
		a.pending = append(a.pending, pendingOpen{
			ev: ev, deadline: time.Now().Add(a.grace), context: a.contextFor(ev.Container),
		})
		return
	}
	s.openAt = ev.Timestamp // mashina datchikdan chiqdi — endi "yo'q"
	switch {
	case !s.posAt.IsZero():
		a.emitOpen(KindPaid, ev, s, nil) // Holat 1
	case isEnter(ev.Gate):
		// Kirish bepul: to'lov ham, pult ham talab qilinmaydi.
		a.emitOpen(KindEntry, ev, s, nil)
	case a.remoteRecent(ev):
		a.emitOpen(KindRemote, ev, s, nil) // Holat 2, pult signali logda bor
	default:
		// Mashina bor, to'lov yo'q. Avto-to'lov kelsa Holat 2, kelmasa Holat 3.
		a.pending = append(a.pending, pendingOpen{
			ev: ev, sess: s, deadline: time.Now().Add(a.autoPay),
			context: a.contextFor(ev.Container),
		})
	}
}

// resolveByPayment — pult bilan ochilgan mashina chiqib ketdi va tizim pul yechdi.
func (a *Analyzer) resolveByPayment(s *session) bool {
	if s == nil {
		return false
	}
	for i, p := range a.pending {
		if p.sess != s {
			continue
		}
		a.pending = append(a.pending[:i], a.pending[i+1:]...)
		a.emitOpen(KindRemote, p.ev, s, nil)
		return true
	}
	return false
}

// adoptPending — ANPR ochilishdan keyin kelgan bo'lsa, uni o'sha ochilishga bog'laydi.
func (a *Analyzer) adoptPending(s *session) {
	for i, p := range a.pending {
		if p.sess != nil {
			continue
		}
		if p.ev.Plate != "" && p.ev.Plate != s.anpr.Plate {
			continue
		}
		if s.anpr.Timestamp.After(p.ev.Timestamp) {
			continue // ANPR ochilishdan keyin — bu boshqa mashina
		}
		s.openAt = p.ev.Timestamp
		if p.ev.Type == parser.EventPOS {
			// To'lov so'rovi bor edi, endi ANPR ham topildi — muammosiz o'tish.
			s.posAt = p.ev.Timestamp
			s.passSent = true
			a.pending = append(a.pending[:i], a.pending[i+1:]...)
			a.emitPass(s, p.ev)
			a.emitOpen(KindPaid, p.ev, s, nil)
			return
		}
		// Jismoniy ochilish edi — mashina bor ekan, endi to'lovni kutamiz.
		a.pending[i].sess = s
		a.pending[i].deadline = time.Now().Add(a.autoPay)
		return
	}
}

// suppressed — bu qator allaqachon natijasi chiqarilgan tranzaksiyaga tegishli.
// Raqam butun tranzaksiyani belgilaydi (uzun oyna); raqamsiz qator faqat
// darvoza bo'yicha juftlanadi, shuning uchun oynasi bitta ochilish davri.
func (a *Analyzer) suppressed(ev *parser.Event) bool {
	if ev.Plate != "" {
		if t, ok := a.lastOutcome[plateKey(ev.Plate, ev.Gate)]; ok && absDur(ev.Timestamp.Sub(t)) < a.dedupe {
			return true
		}
	}
	if ev.Gate != "" {
		if t, ok := a.lastOutcome[gateKey(ev.Gate)]; ok && absDur(ev.Timestamp.Sub(t)) < a.gateDedupe {
			return true
		}
	}
	return false
}

// pendingDup — hali hal qilinmagan ochilishning takroriy log qatori.
func (a *Analyzer) pendingDup(ev *parser.Event) bool {
	for _, p := range a.pending {
		if ev.Plate != "" && p.ev.Plate == ev.Plate {
			return true
		}
		if ev.Gate != "" && p.ev.Gate == ev.Gate &&
			absDur(ev.Timestamp.Sub(p.ev.Timestamp)) < a.gateDedupe {
			return true
		}
	}
	return false
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// remoteRecent — shu darvozada ochilishdan sal oldin pult bosilganmi?
func (a *Analyzer) remoteRecent(ev *parser.Event) bool {
	for _, gate := range []string{ev.Gate, ""} {
		if t, ok := a.remoteAt[gate]; ok {
			if d := ev.Timestamp.Sub(t); d >= 0 && d <= a.grace+2*time.Second {
				return true
			}
		}
	}
	return false
}

func (a *Analyzer) contextFor(container string) []string {
	if a.ContextFn == nil {
		return nil
	}
	return a.ContextFn(container)
}

func gateKey(gate string) string { return "gate:" + gate }

// plateKey — tranzaksiya kaliti. Raqamning o'zi yetarli emas: bitta mashina
// kirib, oynadan tezroq chiqib ketsa, chiqishi kirishning duplikati bo'lib
// qolardi. Shuning uchun darvoza ham kalitga kiradi.
func plateKey(plate, gate string) string {
	if gate == "" {
		return plate
	}
	return plate + "@" + gate
}

// sessionFor hodisani sessiyaga bog'laydi: raqami bo'lsa — aynan o'sha,
// bo'lmasa datchikda hali turgan eng so'nggi mashina.
func (a *Analyzer) sessionFor(ev *parser.Event) *session {
	if ev.Plate != "" {
		if s, ok := a.sessions[ev.Plate]; ok && a.inWindow(s.anpr, ev) {
			return s
		}
		return nil
	}
	return a.latestPresent(ev)
}

// latestPresent — hali ochilish ko'rmagan, presence oynasidagi eng so'nggi mashina.
// Raqamsiz hodisalar (RelayWorker, gateway qadamlari) shunga bog'lanadi.
func (a *Analyzer) latestPresent(ev *parser.Event) *session {
	var best *session
	for _, s := range a.sessions {
		if !s.openAt.IsZero() {
			continue // bu mashina allaqachon chiqib ketgan
		}
		if d := ev.Timestamp.Sub(s.anpr.Timestamp); d < 0 || d > a.presence {
			continue
		}
		if best == nil || s.anpr.Timestamp.After(best.anpr.Timestamp) {
			best = s
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
		if now.Before(p.deadline) {
			kept = append(kept, p)
			continue
		}
		switch {
		case p.sess == nil:
			// ANPR kelmadi: datchikda mashina yo'q edi.
			a.emitOpen(KindGhost, p.ev, nil, p.context)
		case !p.sess.posAt.IsZero():
			// Ochilishdan keyin to'lov keldi — demak pult bilan ochilgan.
			a.emitOpen(KindRemote, p.ev, p.sess, nil)
		default:
			a.emitOpen(KindViolation, p.ev, p.sess, p.context)
		}
	}
	a.pending = kept

	// Eski yozuvlarni tozalash (xotira oqib ketmasin)
	for plate, s := range a.sessions {
		if now.Sub(s.seenAt) > a.window+time.Minute {
			delete(a.sessions, plate)
		}
	}
	// lastOutcome va remoteAt log vaqtida yashaydi — a.clock bilan tozalanadi.
	for key, t := range a.lastOutcome {
		if a.clock.Sub(t) > a.dedupe+time.Minute {
			delete(a.lastOutcome, key)
		}
	}
	for gate, t := range a.remoteAt {
		if a.clock.Sub(t) > time.Minute {
			delete(a.remoteAt, gate)
		}
	}
	cutoff := now.Add(-trafficHours * time.Hour).Unix()
	for k := range a.traffic {
		if k < cutoff {
			delete(a.traffic, k)
		}
	}
}

func (a *Analyzer) emitPass(s *session, pos *parser.Event) {
	total := durMs(pos.Timestamp.Sub(s.anpr.Timestamp))
	// Ochilish to'lovdan oldin bo'lgan bo'lsa — pult bilan ochilgan, to'lov
	// mashina ketayotganda olindi. Bu tizim latency'si emas.
	autoPay := !s.openAt.IsZero() && s.openAt.Before(pos.Timestamp)
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
			PosMs:     durMs(pos.Timestamp.Sub(s.permitAt)),
		}
	}
	a.stats.TotalPasses++
	if !autoPay {
		a.sumMs += total
		a.latencyN++
		a.stats.AvgLatencyMs = a.sumMs / float64(a.latencyN)
	}
	a.Out <- Message{Type: "pass", Data: PassEvent{
		Plate: s.anpr.Plate, Gate: pos.Gate,
		AnprAt: s.anpr.Timestamp, RelayAt: pos.Timestamp,
		LatencyMs: total, Breakdown: br, AutoPay: autoPay,
	}}
	a.emitStats()
}

func (a *Analyzer) emitOpen(kind OpenKind, ev *parser.Event, s *session, ctx []string) {
	plate := ev.Plate
	if plate == "" && s != nil {
		plate = s.anpr.Plate
	}
	// Ikkala kalitni ham belgilaymiz: POS qatorida raqam bor, undan keyingi
	// apparat qatorida faqat darvoza bor — ikkovi bitta ochilish.
	if plate != "" {
		a.lastOutcome[plateKey(plate, ev.Gate)] = ev.Timestamp
	}
	if ev.Gate != "" {
		a.lastOutcome[gateKey(ev.Gate)] = ev.Timestamp
	}

	raw := ev.Raw
	if kind.Suspicious() {
		a.stats.GhostCount++
	} else {
		raw, ctx = "", nil // muammosiz holatlar uchun log yozilmaydi
	}
	a.stats.Opens[string(kind)]++
	if kind != KindGhost {
		a.addTraffic(ev.Timestamp, ev.Gate) // arvohda mashina yo'q — sanamaymiz
	}

	a.Out <- Message{Type: "open", Data: OpenEvent{
		Kind: kind, Plate: plate, Gate: ev.Gate, OpenAt: ev.Timestamp,
		Reason: reasonOf(kind), Raw: raw, Context: ctx,
	}}
	a.emitStats()
	a.emitTraffic()
}

func (a *Analyzer) addTraffic(ts time.Time, gate string) {
	h := ts.UTC().Truncate(time.Hour)
	p := a.traffic[h.Unix()]
	if p == nil {
		p = &TrafficPoint{Hour: h}
		a.traffic[h.Unix()] = p
	}
	switch {
	case strings.HasPrefix(gate, "enter"):
		p.Enter++
	case strings.HasPrefix(gate, "exit"):
		p.Exit++
	}
}

// Traffic oxirgi 24 soatning soatlik qatorini qaytaradi (bo'sh soatlar 0 bilan).
func (a *Analyzer) Traffic() []TrafficPoint {
	end := time.Now().UTC().Truncate(time.Hour)
	out := make([]TrafficPoint, 0, trafficHours)
	for i := trafficHours - 1; i >= 0; i-- {
		h := end.Add(-time.Duration(i) * time.Hour)
		if p := a.traffic[h.Unix()]; p != nil {
			out = append(out, *p)
		} else {
			out = append(out, TrafficPoint{Hour: h})
		}
	}
	return out
}

func durMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func (a *Analyzer) emitStats() {
	// Xaritani nusxalaymiz — hub uni boshqa goroutine'da JSON'ga o'giradi.
	opens := make(map[string]int, len(a.stats.Opens))
	for k, v := range a.stats.Opens {
		opens[k] = v
	}
	s := a.stats
	s.Opens = opens
	a.Out <- Message{Type: "stats", Data: s}
}

func (a *Analyzer) emitTraffic() {
	a.Out <- Message{Type: "traffic", Data: a.Traffic()}
}
