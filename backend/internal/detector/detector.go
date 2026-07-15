// Package detector loglarni "aqlli" o'qishning mantiqiy (AI'siz) qatlami.
//
//  1. Har qatorni SHABLONga aylantiradi (raqam/plate/vaqtni '#' bilan almashtirib):
//     "Relay exit 1: opened" va "Relay exit 2: opened" -> bitta shablon.
//  2. To'lov (POS) hodisasidan keyin ~oynada MUNTAZAM kelgan tanilmagan shablonni
//     "ochilish" deb O'RGANADI (vaqt korrelyatsiyasi). Regexga tayanmaydi.
//  3. O'rganilgandan keyin o'sha shablonli qatorlardan sintetik OPEN yasaydi va
//     yo'nalishni KONTEKST bilan aniqlaydi: to'lov ergashgan -> chiqish,
//     faqat ANPR bo'lgan -> kirish.
//
// Shu tufayli darvoza so'zi boshqacha bo'lgan obyektlarda ham grafik/ochilishlar
// avtomatik to'ldiriladi.
package detector

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"parkpulse/backend/internal/parser"
)

// Line — inspector uchun bitta log qatori va uning yorlig'i.
type Line struct {
	Time      time.Time `json:"time"`
	Container string    `json:"container"`
	Text      string    `json:"text"`
	Kind      string    `json:"kind"` // ANPR/POS/OPEN/REMOTE/GATEWAY/PERMIT/NOISE/"" | OPEN* (sintetik)
	Plate     string    `json:"plate,omitempty"`
	Gate      string    `json:"gate,omitempty"`
}

// Learned — avtomatik o'rganilgan ochilish shabloni (UI'da ko'rsatiladi).
type Learned struct {
	Container string  `json:"container"`
	Template  string  `json:"template"`
	Count     int     `json:"count"`
	Ratio     float64 `json:"ratio"`
	Sample    string  `json:"sample"`
}

type tstat struct {
	total    int
	afterPay int
	sample   string
}

type cstate struct {
	lastPay     time.Time
	lastPayGate string
	lastAnpr    time.Time
	tmpl        map[string]*tstat
	openTmpl    string
	openSample  string
	openCount   int
	openRatio   float64
}

type Detector struct {
	mu     sync.Mutex
	window time.Duration // to'lov -> ochilish korrelyatsiya oynasi
	minN   int           // o'rganish uchun minimal namuna
	minR   float64       // to'lovdan keyin kelish ulushi (0..1)

	st map[string]*cstate

	ring    []Line
	ringCap int
}

func New() *Detector {
	return &Detector{
		window:  envDurSec("OPEN_LEARN_WINDOW_SEC", 8*time.Second),
		minN:    envInt("OPEN_LEARN_MIN", 5),
		minR:    envFloat("OPEN_LEARN_RATIO", 0.6),
		st:      make(map[string]*cstate),
		ringCap: 300,
	}
}

// Feed collector'dan har qator uchun chaqiriladi. Analyzer'ga yuboriladigan
// hodisalarni qaytaradi: odatdagi ev (bo'lsa) + o'rganilgan shablonli sintetik OPEN.
func (d *Detector) Feed(container, text string, ts time.Time, kind string, ev *parser.Event) []*parser.Event {
	d.mu.Lock()
	defer d.mu.Unlock()

	cs := d.state(container)
	label := kind
	var plate, gate string
	if ev != nil {
		plate, gate = ev.Plate, ev.Gate
	}
	var out []*parser.Event

	switch kind {
	case "POS":
		cs.lastPay = ts
		if gate != "" {
			cs.lastPayGate = gate
		}
	case "ANPR":
		cs.lastAnpr = ts
	}

	switch {
	case ev != nil:
		out = append(out, ev)

	case kind == "": // tanilmagan qator — o'rganish yoki sintetik open
		tmpl := templatize(text)
		if cs.openTmpl != "" && tmpl == cs.openTmpl {
			g := d.inferGate(cs, ts, text)
			se := &parser.Event{
				Type: parser.EventOpen, Timestamp: ts, Container: container,
				Gate: g, Plate: parser.ExtractPlate(text), Raw: text,
			}
			out = append(out, se)
			label, gate, plate = "OPEN*", g, se.Plate
		} else {
			d.learn(cs, tmpl, text, ts)
		}
	}

	d.record(Line{Time: ts, Container: container, Text: text, Kind: label, Plate: plate, Gate: gate})
	return out
}

// learn tanilmagan shablonni to'lov bilan korrelyatsiyasi bo'yicha baholaydi.
func (d *Detector) learn(cs *cstate, tmpl, text string, ts time.Time) {
	if len(cs.tmpl) > 512 { // xotira o'sib ketmasin — kam uchraganlarni tozalaymiz
		for k, v := range cs.tmpl {
			if v.total < 2 {
				delete(cs.tmpl, k)
			}
		}
	}
	tsx := cs.tmpl[tmpl]
	if tsx == nil {
		tsx = &tstat{sample: text}
		cs.tmpl[tmpl] = tsx
	}
	tsx.total++
	if !cs.lastPay.IsZero() && ts.Sub(cs.lastPay) >= 0 && ts.Sub(cs.lastPay) <= d.window {
		tsx.afterPay++
	}
	if cs.openTmpl == "" && tsx.afterPay >= d.minN {
		if r := float64(tsx.afterPay) / float64(tsx.total); r >= d.minR {
			cs.openTmpl, cs.openSample = tmpl, tsx.sample
			cs.openCount, cs.openRatio = tsx.afterPay, r
		}
	}
}

// inferGate sintetik ochilishning yo'nalishini kontekstdan aniqlaydi.
func (d *Detector) inferGate(cs *cstate, ts time.Time, text string) string {
	if g := parser.GateOf(text); g != "" {
		return g // qatorning o'zida darvoza so'zi bor
	}
	if !cs.lastPay.IsZero() && ts.Sub(cs.lastPay) >= 0 && ts.Sub(cs.lastPay) <= d.window {
		if cs.lastPayGate != "" {
			return cs.lastPayGate // to'lov ergashgan -> chiqish
		}
		return "exit 1"
	}
	if !cs.lastAnpr.IsZero() && ts.Sub(cs.lastAnpr) >= 0 && ts.Sub(cs.lastAnpr) <= d.window {
		return "enter 1" // faqat ANPR (to'lovsiz) -> kirish
	}
	return "exit 1"
}

func (d *Detector) state(container string) *cstate {
	cs := d.st[container]
	if cs == nil {
		cs = &cstate{tmpl: make(map[string]*tstat)}
		d.st[container] = cs
	}
	return cs
}

func (d *Detector) record(l Line) {
	d.ring = append(d.ring, l)
	if len(d.ring) > d.ringCap {
		d.ring = d.ring[len(d.ring)-d.ringCap:]
	}
}

// Lines inspektor uchun oxirgi qatorlar nusxasini qaytaradi (eng yangisi oxirida).
func (d *Detector) Lines() []Line {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Line, len(d.ring))
	copy(out, d.ring)
	return out
}

// Learned har konteyner uchun o'rganilgan ochilish shablonini qaytaradi.
func (d *Detector) Learned() []Learned {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []Learned
	for name, cs := range d.st {
		if cs.openTmpl != "" {
			out = append(out, Learned{
				Container: name, Template: cs.openTmpl, Count: cs.openCount,
				Ratio: cs.openRatio, Sample: cs.openSample,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Container < out[j].Container })
	return out
}

var (
	// Shablon uchun: aralash harf+raqam (plate), sof raqam -> '#'
	reMixed = regexp.MustCompile(`\b(?:[A-Za-z]+\d|\d+[A-Za-z])[A-Za-z0-9]*\b`)
	reNum   = regexp.MustCompile(`\d+`)
)

// templatize qatorni o'zgaruvchan qismlarsiz imzoga aylantiradi.
func templatize(s string) string {
	s = reMixed.ReplaceAllString(s, "#")
	s = reNum.ReplaceAllString(s, "#")
	return strings.Join(strings.Fields(s), " ")
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && v > 0 {
		return v
	}
	return def
}

func envDurSec(key string, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return def
}
