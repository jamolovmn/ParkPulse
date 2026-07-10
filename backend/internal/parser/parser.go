// Package parser log qatorlarini RegEx orqali tahlil qilib, Event'ga aylantiradi.
// RegEx'lar haqiqiy p24 log formatiga moslangan:
//
//	ANPR:   "20260703 12:59:02.065187 UTC 1 DEBUG [operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc:178"
//	POS:    "20260703 13:00:28.395886 UTC 1 DEBUG [makePayment] Vendotek exit 1: Requesting payment: 01M635ZB (20000) - POSWorker.cpp:67"
//	        "20260703 12:58:35.552016 UTC 1 DEBUG [handleCommand] Vendotek exit 1: The uid is already being processed: 01M635ZB - POSWorker.cpp:44"
//	OPEN:   "20260703 13:00:29.100000 UTC 1 DEBUG [openGate] Relay exit 1:  - RelayWorker.cpp:33"
//
// Muhim farq: POS = "to'lov so'raldi" (pul), OPEN = "shlagbaum jismonan ochildi"
// (temir). Ilgari ikkalasi bitta EventRelay edi va RelayWorker'ning HAR QANDAY
// qatori (jumladan "Connection is closed") ochilish deb hisoblanardi — arvoh
// ochilishlar shundan kelib chiqardi.
package parser

import (
	"os"
	"regexp"
	"strings"
	"time"
)

type EventType string

// Zanjir (real p24 loglaridan): ANPR (1) -> GATEWAY (2) -> PERMIT/DB (3) -> POS (4)
// OPEN va REMOTE zanjirdan tashqarida — shlagbaumning jismoniy holati.
const (
	EventANPR    EventType = "ANPR"    // 1-qadam: raqam o'qildi
	EventGateway EventType = "GATEWAY" // 2-qadam: gateway ishga tushdi
	EventPermit  EventType = "PERMIT"  // 3-qadam: DB javobi (permit topildi/yaratildi)
	EventPOS     EventType = "POS"     // 4-qadam: POS'ga to'lov so'rovi
	EventOpen    EventType = "OPEN"    // shlagbaum jismonan ochildi (RelayWorker)
	EventRemote  EventType = "REMOTE"  // qorovul pultni bosdi
)

type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Container string    `json:"container"`
	Plate     string    `json:"plate,omitempty"` // 01M635ZB
	Gate      string    `json:"gate,omitempty"`  // "exit 1", "enter 1"
	Raw       string    `json:"raw"`
}

var (
	// ANPR: chiziqlar orasidagi davlat raqami: "-------- 01M635ZB --------"
	reANPR = regexp.MustCompile(`-{3,}\s*([A-Z0-9]{5,10})\s*-{3,}`)
	// 2-qadam (Gateway): "In flight mode started" (yangi so'rov) yoki
	// "Recent permit found and assigned" (kesh'dan) — ANPR'dan ~0.1-0.2ms keyin
	reGateway = regexp.MustCompile(`(?i)\bIn flight mode started\b|\bRecent permit found and assigned\b`)
	// 3-qadam (DB): permit topildi yoki yaratildi
	rePermit = regexp.MustCompile(`(?i)\bCurrent permit found and assigned\b|\bPermit(?: visit)? created\b`)
	// 4-qadam (POS): FAQAT to'lov so'rovlari. "Processing payment", "Idle state",
	// "VRP canceled" ham "Vendotek exit" bilan boshlanadi — ular to'lov EMAS.
	rePOS = regexp.MustCompile(`(?i)\b(?:Vendotek|QR)\s+((?:enter|exit)\s+\d+):\s*(?:Requesting payment|The uid is already being processed)`)

	// Shlagbaumning jismoniy ochilishi. RelayWorker qatori ochilish deb faqat
	// tanasi bo'sh bo'lsa ("Relay exit 1:  - RelayWorker.cpp") yoki ochish
	// fe'li bo'lsa hisoblanadi. Aks holda bu shunchaki apparat holati.
	// RELAY_OPEN_RE env orqali obyekt log formatiga moslanadi (darvoza uchun
	// birinchi guruh bo'lsa ishlatiladi, bo'lmasa matndan topiladi).
	reOpen = envRegexp("RELAY_OPEN_RE",
		`(?i)\bRelay\s+((?:enter|exit)\s+\d+):\s*(?:-\s*RelayWorker\.cpp|(?:Open(?:ed|ing)?|Switch(?:ed)?\s+on|Impulse|Pulse)\b)`)

	// RelayWorker/Vendotek shovqini: ulanish xatolari, qayta urinishlar.
	// "Connection is closed" tarmoq xatosi — darvoza ochilishi EMAS.
	reRelayNoise = regexp.MustCompile(`(?i)\bConnection\s+(?:is\s+closed|lost|refused|failed|error)\b|\bReconnect|\bTimed?\s?out\b|\bDisconnected\b|\bHeartbeat\b`)

	// Pult (qorovul tugmasi). Obyektlarda log matni har xil — RELAY_REMOTE_RE
	// env orqali almashtiriladi. Signal bo'lmasa analyzer avto-to'lov
	// evristikasiga tayanadi, shuning uchun bu majburiy emas.
	// p24 loglarida "[operator()]" — bu C++ funksiya nomi, qorovul emas.
	// Shuning uchun "operator" faqat "opened by operator" shaklida qabul qilinadi.
	reRemote = envRegexp("RELAY_REMOTE_RE",
		`(?i)\b(?:remote|manual|pult)\b[^\n]*\bopen|\bOpen(?:ed|ing)?\s+(?:by|from)\s+(?:operator|guard|remote|pult|button)\b`)

	// Darvoza nomi: "exit 1", "enter 2"
	reGate = regexp.MustCompile(`(?i)\b(enter|exit)\s+(\d+)\b`)
	// Nomzod token: harf ham, raqam ham qatnashgan 5-10 belgi (pastda filtrlanadi)
	rePlateToken = regexp.MustCompile(`\b([A-Z0-9]+)\b`)
	// Dastur o'z logidagi vaqt: "20260703 12:59:02.065187 UTC"
	reAppTS = regexp.MustCompile(`^(\d{8} \d{2}:\d{2}:\d{2}\.\d+ [A-Z]+)\s*`)
	// Docker "--timestamps" rejimida qator boshiga qo'shiladigan RFC3339Nano vaqt
	reDockerTS = regexp.MustCompile(`^(\S+)\s`)
)

const appTSLayout = "20060102 15:04:05.000000 MST"

func envRegexp(key, def string) *regexp.Regexp {
	if v := os.Getenv(key); v != "" {
		if re, err := regexp.Compile(v); err == nil {
			return re
		}
	}
	return regexp.MustCompile(def)
}

// Clean Docker "--timestamps" prefiksini olib tashlaydi (kontekst ko'rinishi uchun).
func Clean(line string) string {
	line = strings.TrimRight(line, "\r\n")
	if m := reDockerTS.FindStringSubmatch(line); m != nil {
		if _, err := time.Parse(time.RFC3339Nano, m[1]); err == nil {
			return strings.TrimPrefix(line, m[0])
		}
	}
	return line
}

// Parse bitta log qatorini tekshiradi. Tanish hodisa bo'lmasa nil qaytaradi.
// Vaqt ustuvorligi: dastur o'z vaqti (eng aniq) -> Docker vaqti -> hozirgi vaqt.
func Parse(container, line string) *Event {
	line = strings.TrimRight(line, "\r\n")

	ts := time.Now()
	msg := line
	if m := reDockerTS.FindStringSubmatch(line); m != nil {
		if t, err := time.Parse(time.RFC3339Nano, m[1]); err == nil {
			ts = t
			msg = strings.TrimPrefix(line, m[0])
		}
	}
	if m := reAppTS.FindStringSubmatch(msg); m != nil {
		if t, err := time.Parse(appTSLayout, m[1]); err == nil {
			ts = t
		}
	}

	if m := reANPR.FindStringSubmatch(msg); m != nil {
		return &Event{Type: EventANPR, Timestamp: ts, Container: container, Plate: strings.ToUpper(m[1]), Raw: msg}
	}
	if reGateway.MatchString(msg) {
		return midStep(EventGateway, ts, container, msg)
	}
	if rePermit.MatchString(msg) {
		return midStep(EventPermit, ts, container, msg)
	}
	// Apparat shovqini hech qachon ochilish emas — qolgan qoidalardan oldin kesamiz.
	if reRelayNoise.MatchString(msg) {
		return nil
	}
	if m := rePOS.FindStringSubmatch(msg); m != nil {
		return &Event{
			Type: EventPOS, Timestamp: ts, Container: container,
			Gate: normGate(m[1]), Plate: extractPlate(msg), Raw: msg,
		}
	}
	if m := reOpen.FindStringSubmatch(msg); m != nil {
		gate := ""
		if len(m) > 1 {
			gate = normGate(m[1])
		}
		if gate == "" {
			gate = gateOf(msg) // maxsus regexda guruh bo'lmasa
		}
		return &Event{
			Type: EventOpen, Timestamp: ts, Container: container,
			Gate: gate, Plate: extractPlate(msg), Raw: msg,
		}
	}
	if reRemote.MatchString(msg) {
		return &Event{Type: EventRemote, Timestamp: ts, Container: container, Gate: gateOf(msg), Raw: msg}
	}
	return nil
}

// midStep oraliq qadam (GATEWAY/PERMIT) hodisasini yasaydi; qatorda raqam
// bo'lsa oladi, bo'lmasa analyzer eng so'nggi ochiq sessiyaga bog'laydi.
func midStep(t EventType, ts time.Time, container, msg string) *Event {
	return &Event{Type: t, Timestamp: ts, Container: container, Plate: extractPlate(msg), Raw: msg}
}

func normGate(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func gateOf(msg string) string {
	if m := reGate.FindStringSubmatch(msg); m != nil {
		return normGate(m[1] + " " + m[2])
	}
	return ""
}

func extractPlate(msg string) string {
	for _, m := range rePlateToken.FindAllStringSubmatch(msg, -1) {
		p := m[1]
		if len(p) < 5 || len(p) > 10 {
			continue
		}
		var hasLetter, hasDigit bool
		for _, r := range p {
			if r >= 'A' && r <= 'Z' {
				hasLetter = true
			}
			if r >= '0' && r <= '9' {
				hasDigit = true
			}
		}
		// Raqam bo'lishi uchun harf HAM, raqam HAM qatnashishi shart —
		// bu "DEBUG", "WARN" kabi so'zlarni xato tanishni oldini oladi.
		if hasLetter && hasDigit {
			return p
		}
	}
	return ""
}
