// Package parser log qatorlarini RegEx orqali tahlil qilib, Event'ga aylantiradi.
// RegEx'lar haqiqiy p24 log formatiga moslangan:
//
//	ANPR:  "20260703 13:01:11.333226 UTC 1 DEBUG [operator()] ------------- 01M635ZB ------------- - GatewayPlugin.cc:178"
//	Relay: "20260703 13:01:10.254157 UTC 138 WARN  Relay exit 2: Connection is closed - RelayWorker.cpp:57"
package parser

import (
	"regexp"
	"strings"
	"time"
)

type EventType string

const (
	EventANPR  EventType = "ANPR"  // Raqam o'qildi
	EventRelay EventType = "RELAY" // Shlagbaum (relay) hodisasi
)

type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Container string    `json:"container"`
	Plate     string    `json:"plate,omitempty"` // ANPR: 01M635ZB
	Gate      string    `json:"gate,omitempty"`  // RELAY: "exit 2", "enter 1"
	Raw       string    `json:"raw"`
}

var (
	// ANPR: chiziqlar orasidagi davlat raqami: "------- 01M635ZB -------"
	rePlate = regexp.MustCompile(`-{3,}\s*([A-Z0-9]{5,10})\s*-{3,}`)
	// Relay: "Relay" so'zidan keyingi darvoza nomi: "Relay exit 2:", "Relay enter 1:"
	reRelay = regexp.MustCompile(`(?i)\bRelay\s+([a-z]+(?:\s+\d+)?)`)
	// Dastur o'z logidagi vaqt: "20260703 13:01:11.333226 UTC"
	reAppTS = regexp.MustCompile(`^(\d{8} \d{2}:\d{2}:\d{2}\.\d+ [A-Z]+)\s*`)
	// Docker "--timestamps" rejimida qator boshiga qo'shadigan RFC3339Nano vaqt
	reDockerTS = regexp.MustCompile(`^(\S+)\s`)
)

const appTSLayout = "20060102 15:04:05.000000 MST"

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

	if m := rePlate.FindStringSubmatch(msg); m != nil {
		return &Event{Type: EventANPR, Timestamp: ts, Container: container, Plate: strings.ToUpper(m[1]), Raw: msg}
	}
	if m := reRelay.FindStringSubmatch(msg); m != nil {
		gate := strings.ToLower(strings.Join(strings.Fields(m[1]), " "))
		return &Event{Type: EventRelay, Timestamp: ts, Container: container, Gate: gate, Raw: msg}
	}
	return nil
}
