package netmon

import (
	"path/filepath"
	"testing"
)

func watchedOf(devs []Device, ip string) (bool, bool) {
	for _, d := range devs {
		if d.IP == ip {
			return d.Watched, true
		}
	}
	return false, false
}

func nameOf(devs []Device, ip string) string {
	for _, d := range devs {
		if d.IP == ip {
			return d.Name
		}
	}
	return ""
}

func TestWatchDefaultsAndPersist(t *testing.T) {
	store := filepath.Join(t.TempDir(), "devices.json")
	t.Setenv("DEVICES", "cam=10.0.0.5")
	t.Setenv("DEVICES_STORE", store)

	m := New()
	// Skanerda topilgandek qurilma (config'da yo'q — pinned false).
	m.devices["10.0.0.9"] = &Device{Name: "phone", IP: "10.0.0.9"}

	devs := m.Devices()
	if w, ok := watchedOf(devs, "10.0.0.5"); !ok || !w {
		t.Error("config qurilmasi standart bo'yicha kuzatilishi kerak (★)")
	}
	if w, _ := watchedOf(devs, "10.0.0.9"); w {
		t.Error("skanerdagi qurilma standart bo'yicha kuzatilmasligi kerak (☆)")
	}

	// UI'dan: telefonni yoqamiz, kamerani o'chiramiz.
	m.SetWatch("10.0.0.9", true)
	m.SetWatch("10.0.0.5", false)
	if w, _ := watchedOf(m.Devices(), "10.0.0.5"); w {
		t.Error("o'chirilgan config qurilmasi kuzatilmasligi kerak")
	}

	// Restart: tanlov fayldan tiklanadi.
	m2 := New()
	if w, ok := watchedOf(m2.Devices(), "10.0.0.5"); !ok || w {
		t.Error("restartda 'o'chirilgan' tanlov saqlanmadi")
	}
	if o := m2.over["10.0.0.9"]; o == nil || o.Watched == nil || !*o.Watched {
		t.Error("restartda telefon tanlovi saqlanmadi")
	}
}

func TestSetNamePersists(t *testing.T) {
	store := filepath.Join(t.TempDir(), "devices.json")
	t.Setenv("DEVICES", "")
	t.Setenv("DEVICES_STORE", store)

	m := New()
	m.devices["10.0.0.9"] = &Device{Name: "Oddiy qurilma", IP: "10.0.0.9"}
	m.SetName("10.0.0.9", "Relay")
	if n := nameOf(m.Devices(), "10.0.0.9"); n != "Relay" {
		t.Errorf("nom 'Relay' bo'lishi kerak, %q chiqdi", n)
	}

	// Restartda saqlanadi; skaner avto-nomi ustidan yozadi.
	m2 := New()
	m2.devices["10.0.0.9"] = &Device{Name: "Oddiy qurilma", IP: "10.0.0.9"}
	if n := nameOf(m2.Devices(), "10.0.0.9"); n != "Relay" {
		t.Errorf("restartda qo'lda nom saqlanmadi: %q", n)
	}

	// Bo'sh nom — avtomatik nomga qaytaradi.
	m2.SetName("10.0.0.9", "")
	if n := nameOf(m2.Devices(), "10.0.0.9"); n != "Oddiy qurilma" {
		t.Errorf("bo'sh nomdan keyin avto-nom kutildi, %q chiqdi", n)
	}
}
