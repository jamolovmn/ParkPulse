package alert

import (
	"path/filepath"
	"testing"

	"parkpulse/backend/internal/analyzer"
	"parkpulse/backend/internal/netmon"
)

// testManager — sinklarsiz, faqat navbatni tekshirish uchun.
func testManager() *Manager {
	m := New()
	m.webhook = "http://example.invalid" // Enabled() true bo'lsin (deliver chaqirilmaydi)
	return m
}

func TestDeviceTransitionAlerts(t *testing.T) {
	m := testManager()
	cam := func(alive bool) analyzer.Message {
		return analyzer.Message{Data: []netmon.Device{{IP: "1.1.1.1", Name: "cam", Alive: alive, Watched: true}}}
	}
	// Birinchi ko'rish — baseline, xabar chiqmasligi kerak.
	m.Observe(cam(true))
	if len(m.queue) != 0 {
		t.Fatalf("birinchi ko'rishda xabar bo'lmasligi kerak, %d chiqdi", len(m.queue))
	}
	// Uzildi — bitta critical xabar.
	m.Observe(cam(false))
	if len(m.queue) != 1 {
		t.Fatalf("uzilishda 1 xabar kutildi, %d chiqdi", len(m.queue))
	}
	n := <-m.queue
	if n.Level != "critical" {
		t.Errorf("critical kutildi, %q chiqdi", n.Level)
	}
	// O'zgarish yo'q — yangi xabar yo'q.
	m.Observe(cam(false))
	if len(m.queue) != 0 {
		t.Errorf("holat o'zgarmaganda xabar bo'lmasligi kerak, %d chiqdi", len(m.queue))
	}
}

func TestUnwatchedDeviceNoAlert(t *testing.T) {
	m := testManager()
	// Kuzatilmaydigan qurilma (telefon) uzilsa — xabar bo'lmasligi kerak.
	m.Observe(analyzer.Message{Data: []netmon.Device{{IP: "2.2.2.2", Name: "phone", Alive: true, Watched: false}}})
	m.Observe(analyzer.Message{Data: []netmon.Device{{IP: "2.2.2.2", Name: "phone", Alive: false, Watched: false}}})
	if len(m.queue) != 0 {
		t.Errorf("kuzatilmaydigan qurilmada xabar bo'lmasligi kerak, %d chiqdi", len(m.queue))
	}
}

func TestSuspiciousOpenAlerts(t *testing.T) {
	m := testManager()
	// Muammosiz ochilish — xabar yo'q.
	m.Observe(analyzer.Message{Data: analyzer.OpenEvent{Kind: analyzer.KindPaid, Gate: "exit 1"}})
	if len(m.queue) != 0 {
		t.Fatalf("paid ochilishda xabar bo'lmasligi kerak, %d chiqdi", len(m.queue))
	}
	// Arvoh — critical xabar.
	m.Observe(analyzer.Message{Data: analyzer.OpenEvent{Kind: analyzer.KindGhost, Gate: "exit 1", Reason: "test"}})
	if len(m.queue) != 1 {
		t.Fatalf("arvohda 1 xabar kutildi, %d chiqdi", len(m.queue))
	}
	if n := <-m.queue; n.Level != "critical" {
		t.Errorf("critical kutildi, %q chiqdi", n.Level)
	}
}

func TestEnabled(t *testing.T) {
	m := New() // env bo'sh
	if m.Enabled() {
		t.Error("sinksiz Enabled()=false bo'lishi kerak")
	}
}

func TestSetConfigPersistsAndLoads(t *testing.T) {
	store := filepath.Join(t.TempDir(), "alerts.json")

	// 1-instans: UI orqali sozlaydi va saqlaydi.
	m1 := New()
	m1.store = store
	if err := m1.SetConfig(Settings{TelegramToken: "123:ABC", TelegramChat: "-100"}); err != nil {
		t.Fatalf("SetConfig xato: %v", err)
	}
	if !m1.Enabled() {
		t.Fatal("saqlangandan keyin Enabled() true bo'lishi kerak")
	}

	// 2-instans (restart): fayldan o'qib oladi.
	m2 := New()
	m2.store = store
	m2.LoadStore()
	got := m2.Config()
	if got.TelegramToken != "123:ABC" || got.TelegramChat != "-100" {
		t.Errorf("restartda sozlama tiklanmadi: %+v", got)
	}
	if !m2.Enabled() {
		t.Error("restartdan keyin Enabled() true bo'lishi kerak")
	}
}
