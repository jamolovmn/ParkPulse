// Package parser log qatorlarini RegEx orqali tahlil qilib, Event'ga aylantiradi.
// RegEx'lar haqiqiy p24 log formatiga moslangan:
//
//	ANPR:  "20260703 12:59:02.065187 UTC 1 DEBUG [operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc:178"
//	Relay: "20260703 13:00:28.395886 UTC 1 DEBUG [makePayment] Vendotek exit 1: Requesting payment: 01M635ZB (20000) - POSWorker.cpp:67"
//	       "20260703 12:58:35.552016 UTC 1 DEBUG [handleCommand] Vendotek exit 1: The uid is already being processed: 01M635ZB - POSWorker.cpp:44"
package parser

import (
	"regexp"
	"strings"
	"time"
)

type EventType string

// Zanjir (real p24 loglaridan): ANPR (1) -> GATEWAY (2) -> PERMIT/DB (3) -> RELAY/POS (4)
const (
	EventANPR    EventType = "ANPR"    // 1-qadam: raqam o'qildi
	EventGateway EventType = "GATEWAY" // 2-qadam: gateway ishga tushdi
	EventPermit  EventType = "PERMIT"  // 3-qadam: DB javobi (permit topildi/yaratildi)
	EventRelay   EventType = "RELAY"   // 4-qadam: POS'ga buyruq (relay ochilishi)
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
	// 4-qadam (POS buyruq): FAQAT ruxsat so'rovlari. "Processing payment",
	// "Idle state", "VRP canceled" kabi terminal holatlari ham "Vendotek exit"
	// bilan boshlanadi — ular relay EMAS, shuning uchun harakat ham tekshiriladi.
	reRelay = regexp.MustCompile(`(?i)\b(?:Vendotek|QR)\s+((?:enter|exit)\s+\d+):\s*(?:Requesting payment|The uid is already being processed)`)
	// O'zbek davlat raqami: 01M635ZB yoki 01777ABC (kamida bitta harf bor,
	// shuning uchun "(20000)" kabi summalarga yopishmaydi)
	rePlateToken = regexp.MustCompile(`\b(\d{2}[A-Z]\d{3}[A-Z]{2}|\d{5}[A-Z]{3})\b`)
	// Dastur o'z logidagi vaqt: "20260703 12:59:02.065187 UTC"
	reAppTS = regexp.MustCompile(`^(\d{8} \d{2}:\d{2}:\d{2}\.\d+ [A-Z]+)\s*`)
	// Docker "--timestamps" rejimida qator boshiga qo'shiladigan RFC3339Nano vaqt
	reDockerTS = regexp.MustCompile(`^(\S+)\s`)
)

const appTSLayout = "20060102 15:04:05.000000 MST"

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

// Parse bitta log qatorini tekshiradi. ANPR/Relay bo'lmasa nil qaytaradi.
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
	if m := reRelay.FindStringSubmatch(msg); m != nil {
		ev := &Event{
			Type: EventRelay, Timestamp: ts, Container: container,
			Gate: strings.ToLower(strings.Join(strings.Fields(m[1]), " ")),
			Raw:  msg,
		}
		// Relay qatorida raqam ham bor — juftlashtirish shu orqali aniq bo'ladi
		if p := rePlateToken.FindStringSubmatch(msg); p != nil {
			ev.Plate = p[1]
		}
		return ev
	}
	return nil
}

// midStep oraliq qadam (GATEWAY/PERMIT) hodisasini yasaydi; qatorda raqam
// bo'lsa oladi, bo'lmasa analyzer eng so'nggi ochiq sessiyaga bog'laydi.
func midStep(t EventType, ts time.Time, container, msg string) *Event {
	ev := &Event{Type: t, Timestamp: ts, Container: container, Raw: msg}
	if p := rePlateToken.FindStringSubmatch(msg); p != nil {
		ev.Plate = p[1]
	}
	return ev
}
