package parser

import (
	"testing"
	"time"
)

// Haqiqiy p24 loglaridan olingan namunalar.
func TestParseANPR(t *testing.T) {
	line := "20260703 13:01:11.333226 UTC 1 DEBUG [operator()] ------------- 01M635ZB ------------- - GatewayPlugin.cc:178"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Type != EventANPR {
		t.Fatalf("ANPR aniqlanmadi: %+v", ev)
	}
	if ev.Plate != "01M635ZB" {
		t.Errorf("plate = %q, kutilgan 01M635ZB", ev.Plate)
	}
	want := time.Date(2026, 7, 3, 13, 1, 11, 333226000, time.UTC)
	if !ev.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, kutilgan %v", ev.Timestamp, want)
	}
}

func TestParseRelay(t *testing.T) {
	line := "20260703 13:01:10.254157 UTC 138 WARN  Relay exit 2: Connection is closed - RelayWorker.cpp:57"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Type != EventRelay {
		t.Fatalf("Relay aniqlanmadi: %+v", ev)
	}
	if ev.Gate != "exit 2" {
		t.Errorf("gate = %q, kutilgan \"exit 2\"", ev.Gate)
	}
}

func TestParseRelayEnter(t *testing.T) {
	line := "20260703 13:01:11.733226 UTC 138 INFO  Relay enter 1: Impulse sent - RelayWorker.cpp:57"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Type != EventRelay || ev.Gate != "enter 1" {
		t.Fatalf("gate xato: %+v", ev)
	}
}

// Docker --timestamps prefiksi bilan ham ishlashi kerak.
func TestParseWithDockerPrefix(t *testing.T) {
	line := "2026-07-03T13:01:11.400000000Z 20260703 13:01:11.333226 UTC 1 DEBUG [operator()] ------------- 01M635ZB ------------- - GatewayPlugin.cc:178"
	ev := Parse("p24gui", line)
	if ev == nil || ev.Plate != "01M635ZB" {
		t.Fatalf("docker prefiks bilan ishlamadi: %+v", ev)
	}
	// Dastur vaqti (333226) docker vaqtidan (400000) ustuvor bo'lishi kerak
	if ev.Timestamp.Nanosecond() != 333226000 {
		t.Errorf("dastur vaqti olinmadi: %v", ev.Timestamp)
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	for _, line := range []string{
		"20260703 13:01:12.000000 UTC 5 INFO  Heartbeat ok - Main.cpp:10",
		"random text without keywords",
	} {
		if ev := Parse("p24gui", line); ev != nil {
			t.Errorf("shovqin hodisa deb olindi: %q -> %+v", line, ev)
		}
	}
}
