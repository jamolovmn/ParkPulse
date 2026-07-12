package snmp

import "testing"

func TestParseTargets(t *testing.T) {
	got := parseTargets("Core=192.168.1.1@public,192.168.1.2,Edge=10.0.0.3@secret#1")
	if len(got) != 3 {
		t.Fatalf("3 ta target kutildi, %d ta chiqdi: %+v", len(got), got)
	}
	if got[0] != (Target{Name: "Core", IP: "192.168.1.1", Community: "public", Version: "2c"}) {
		t.Errorf("target[0] xato: %+v", got[0])
	}
	// Nom berilmagan: IP nomga aylanadi, community standart.
	if got[1] != (Target{Name: "192.168.1.2", IP: "192.168.1.2", Community: "public", Version: "2c"}) {
		t.Errorf("target[1] xato: %+v", got[1])
	}
	// Versiya suffiksi.
	if got[2] != (Target{Name: "Edge", IP: "10.0.0.3", Community: "secret", Version: "1"}) {
		t.Errorf("target[2] xato: %+v", got[2])
	}
}

func TestParseTargetsEmpty(t *testing.T) {
	if len(parseTargets("")) != 0 {
		t.Fatal("bo'sh satr uchun target bo'lmasligi kerak")
	}
}

func TestBps(t *testing.T) {
	// 1_000_000 bayt / 1s * 8 = 8 Mbit/s
	if v := bps(0, 1_000_000, 1); v != 8 {
		t.Errorf("8 Mbps kutildi, %v chiqdi", v)
	}
	// Hisoblagich qayta boshlandi (manfiy farq) -> 0
	if v := bps(100, 50, 1); v != 0 {
		t.Errorf("counter reset -> 0 kutildi, %v chiqdi", v)
	}
}

func TestFormatTicks(t *testing.T) {
	// 100 tick = 1s; 1 kun = 8_640_000 tick
	if s := formatTicks(8_640_000 + 3600*100 + 60*100); s != "1d 1h 1m" {
		t.Errorf("'1d 1h 1m' kutildi, %q chiqdi", s)
	}
	if s := formatTicks(90 * 100); s != "1m" {
		t.Errorf("'1m' kutildi, %q chiqdi", s)
	}
}

func TestLastIndex(t *testing.T) {
	if idx, ok := lastIndex(".1.3.6.1.2.1.2.2.1.8.42"); !ok || idx != 42 {
		t.Errorf("42 kutildi, %d ok=%v", idx, ok)
	}
	if _, ok := lastIndex("noindex"); ok {
		t.Error("indekssiz OID uchun ok=false kutildi")
	}
}
