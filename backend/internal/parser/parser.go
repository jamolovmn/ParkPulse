// Package parser log qatorlarini RegEx orqali tahlil qilib, Event'ga aylantiradi.
package parser

import (
	"regexp"
	"strings"
	"time"
)

type EventType string

const (
	EventANPR  EventType = "ANPR"  // Raqam o'qildi
	EventRelay EventType = "RELAY" // Shlagbaum ochildi
)

type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"` // Docker bergan aniq vaqt (ns)
	Container string    `json:"container"`
	Plate     string    `json:"plate,omitempty"` // ANPR: 01A123BC
	Gate      string    `json:"gate,omitempty"`  // RELAY: gate identifikatori
	Raw       string    `json:"raw"`
}

// Asosiy tizim log formatiga qarab shu ikkita pattern'ni moslashtirasiz.
// Misol loglar:
//   ANPR: plate=01A123BC confidence=0.97 cam=entry-1
//   Relay Open gate=entry-1 reason=anpr_match
var (
	reANPR  = regexp.MustCompile(`(?i)\bANPR\b.*?plate[=:\s]+"?([A-Z0-9]{5,10})"?`)
	reRelay = regexp.MustCompile(`(?i)\bRelay\s+Open\b(?:.*?gate[=:\s]+"?([\w-]+)"?)?`)
	// ANPR qatoridagi kamera/darvoza identifikatori — Relay bilan juftlashtirish kaliti
	reCam = regexp.MustCompile(`(?i)\b(?:cam(?:era)?|gate)[=:\s]+"?([\w-]+)"?`)
)

// dockerTS — "--timestamps" rejimida har qator boshida keladigan RFC3339Nano vaqt.
var dockerTS = regexp.MustCompile(`^(\S+)\s`)

// Parse bitta log qatorini tekshiradi. Relay/ANPR bo'lmasa nil qaytaradi.
func Parse(container, line string) *Event {
	line = strings.TrimRight(line, "\r\n")

	ts := time.Now()
	msg := line
	if m := dockerTS.FindStringSubmatch(line); m != nil {
		if t, err := time.Parse(time.RFC3339Nano, m[1]); err == nil {
			ts = t
			msg = strings.TrimPrefix(line, m[0])
		}
	}

	if m := reANPR.FindStringSubmatch(msg); m != nil {
		ev := &Event{Type: EventANPR, Timestamp: ts, Container: container, Plate: strings.ToUpper(m[1]), Raw: msg}
		if c := reCam.FindStringSubmatch(msg); c != nil {
			ev.Gate = c[1]
		}
		return ev
	}
	if m := reRelay.FindStringSubmatch(msg); m != nil {
		return &Event{Type: EventRelay, Timestamp: ts, Container: container, Gate: m[1], Raw: msg}
	}
	return nil
}
