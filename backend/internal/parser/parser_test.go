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
		"20260703 13:02:01.000001 UTC 1 DEBUG [handleCommand] QR exit 2: Access granted: 01Y765NC - POSWorker.cpp:44":    "exit 2",
		"20260703 13:02:05.000001 UTC 1 DEBUG [handleCommand] Vendotek enter 1: Requesting: 01Y765NC - POSWorker.cpp:44": "enter 1",
	} {
		ev := Parse("p24gui", line)
		if ev == nil || ev.Type != EventRelay || ev.Gate != gate {
			t.Fatalf("gate xato (%q): %+v", line, ev)
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
		"20260703 13:01:10.254157 UTC 138 WARN  Relay exit 2: Connection is closed - RelayWorker.cpp:57", // endi hodisa emas
		"20260703 13:01:12.000000 UTC 5 INFO  Heartbeat ok - Main.cpp:10",
		"random text without keywords",
	} {
		if ev := Parse("p24gui", line); ev != nil {
			t.Errorf("shovqin hodisa deb olindi: %q -> %+v", line, ev)
		}
	}
}
