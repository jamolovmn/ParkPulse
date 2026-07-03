package parser

import (
	"testing"
	"time"
)

// Haqiqiy p24 loglaridan olingan namunalar.
func TestParseANPR(t *testing.T) {
	for line, plate := range map[string]string{
		"20260703 12:59:02.065187 UTC 1 DEBUG [operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc:178": "01M635ZB",
		"20260703 12:59:11.876929 UTC 1 DEBUG [operator()] -------------- 01Y765NC -------------- - GatewayPlugin.cc:178": "01Y765NC",
	} {
		ev := Parse("p24gui", line)
		if ev == nil || ev.Type != EventANPR {
			t.Fatalf("ANPR aniqlanmadi: %q", line)
		}
		if ev.Plate != plate {
			t.Errorf("plate = %q, kutilgan %q", ev.Plate, plate)
		}
	}
	// Timestamp dastur logidan olinishi kerak
	ev := Parse("p24gui", "20260703 12:59:02.065187 UTC 1 DEBUG [operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc:178")
	want := time.Date(2026, 7, 3, 12, 59, 2, 65187000, time.UTC)
	if !ev.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, kutilgan %v", ev.Timestamp, want)
	}
}

func TestParseRelayPayment(t *testing.T) {
	line := "20260703 13:00:28.395886 UTC 1 DEBUG [makePayment] Vendotek exit 1: Requesting payment: 01M635ZB (20000) - POSWorker.cpp:67"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Type != EventRelay {
		t.Fatalf("Relay aniqlanmadi: %+v", ev)
	}
	if ev.Gate != "exit 1" {
		t.Errorf("gate = %q, kutilgan \"exit 1\"", ev.Gate)
	}
	if ev.Plate != "01M635ZB" {
		t.Errorf("plate = %q, kutilgan 01M635ZB (summa 20000 emas!)", ev.Plate)
	}
}

func TestParseRelayUidProcessed(t *testing.T) {
	line := "20260703 12:58:35.552016 UTC 1 DEBUG [handleCommand] Vendotek exit 1: The uid is already being processed: 01M635ZB - POSWorker.cpp:44"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Type != EventRelay || ev.Gate != "exit 1" || ev.Plate != "01M635ZB" {
		t.Fatalf("uid-processed qatori xato: %+v", ev)
	}
}

func TestParseRelayQRAndEnter(t *testing.T) {
	for line, gate := range map[string]string{
		"20260703 14:41:54.697386 UTC 1 DEBUG [handleCommand] QR exit 2: The uid is already being processed: 01H351LC - POSWorker.cpp:44": "exit 2",
		"20260703 13:02:05.000001 UTC 1 DEBUG [makePayment] Vendotek enter 1: Requesting payment: 01Y765NC (20000) - POSWorker.cpp:67":    "enter 1",
	} {
		ev := Parse("p24gui", line)
		if ev == nil || ev.Type != EventRelay || ev.Gate != gate {
			t.Fatalf("gate xato (%q): %+v", line, ev)
		}
	}
}

func TestParseMidSteps(t *testing.T) {
	// Hammasi haqiqiy p24 loglaridan
	for line, typ := range map[string]EventType{
		"20260703 14:40:06.704351 UTC 1 DEBUG [operator()] In flight mode started - GatewayPlugin.cc:196":            EventGateway,
		"20260703 14:40:40.798688 UTC 1 DEBUG [operator()] Recent permit found and assigned - GatewayPlugin.cc:191":  EventGateway,
		"20260703 14:40:06.716335 UTC 1 DEBUG [operator()] Current permit found and assigned - GatewayPlugin.cc:205": EventPermit,
		"20260703 14:40:39.797629 UTC 1 DEBUG [operator()] Permit created - GatewayPlugin.cc:220":                    EventPermit,
		"20260703 14:40:50.332676 UTC 1 DEBUG [operator()] Permit visit created - GatewayPlugin.cc:249":              EventPermit,
	} {
		ev := Parse("p24gui", line)
		if ev == nil || ev.Type != typ {
			t.Errorf("qadam aniqlanmadi (%q): kutilgan %s, olindi %+v", line, typ, ev)
		}
	}
}

// Docker --timestamps prefiksi bilan ham ishlashi kerak.
func TestParseWithDockerPrefix(t *testing.T) {
	line := "2026-07-03T12:59:02.400000000Z 20260703 12:59:02.065187 UTC 1 DEBUG [operator()] -------------- 01M635ZB -------------- - GatewayPlugin.cc:178"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Plate != "01M635ZB" {
		t.Fatalf("docker prefiks bilan ishlamadi: %+v", ev)
	}
	if ev.Timestamp.Nanosecond() != 65187000 {
		t.Errorf("dastur vaqti ustuvor bo'lishi kerak edi: %v", ev.Timestamp)
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	for _, line := range []string{
		// Relay heartbeat xatosi — access hodisasi emas
		"20260703 14:40:04.450749 UTC 138 WARN  Relay exit 2: Connection is closed - RelayWorker.cpp:57",
		// POS terminal holatlari — "Vendotek exit" bor, lekin relay buyruq EMAS
		"20260703 14:40:06.929108 UTC 121 WARN  Vendotek exit 1: Processing payment - POSWorker.cpp:78",
		"20260703 14:40:39.025699 UTC 121 DEBUG [operator()] Vendotek exit 1: VRP canceled - VTKPOSWorker.cpp:239",
		"20260703 14:40:39.931783 UTC 121 WARN  Vendotek exit 1: Idle state - POSWorker.cpp:78",
		"20260703 14:40:59.266141 UTC 1 ERROR 500: Network failure - GatewayPlugin.cc:425",
		"random text without keywords",
	} {
		if ev := Parse("p24gui", line); ev != nil {
			t.Errorf("shovqin hodisa deb olindi: %q -> %+v", line, ev)
		}
	}
}
