package agent

import (
	"path/filepath"
	"testing"
)

func TestPasswordAuth(t *testing.T) {
	store := filepath.Join(t.TempDir(), "agent.json")
	t.Setenv("AGENT_STORE", store)

	m := New()
	if m.AuthEnabled() {
		t.Fatal("parolsiz auth o'chiq bo'lishi kerak")
	}
	if !m.ValidToken("anything") {
		t.Error("auth o'chiqda har qanday token qabul qilinishi kerak")
	}

	// Parol o'rnatamiz.
	if err := m.SetConfig(Settings{Provider: ProviderOpenRouter, APIKey: "k", Password: "hunter2"}); err != nil {
		t.Fatal(err)
	}
	if !m.AuthEnabled() {
		t.Fatal("parol o'rnatilgach auth yoqilishi kerak")
	}
	tok, ok := m.Login("hunter2")
	if !ok || tok == "" {
		t.Fatal("to'g'ri parol token qaytarishi kerak")
	}
	if _, ok := m.Login("wrong"); ok {
		t.Error("noto'g'ri parol rad etilishi kerak")
	}
	if !m.ValidToken(tok) {
		t.Error("login tokeni yaroqli bo'lishi kerak")
	}
	if m.ValidToken("bad") {
		t.Error("soxta token rad etilishi kerak")
	}

	// Restart: fayldan tiklanadi, o'sha token yaroqli qoladi.
	m2 := New()
	m2.LoadStore()
	if !m2.AuthEnabled() {
		t.Fatal("restartda parol saqlanmadi")
	}
	if !m2.ValidToken(tok) {
		t.Error("restartdan keyin token yaroqli qolishi kerak (auth_secret saqlangan)")
	}
	if _, ok := m2.Login("hunter2"); !ok {
		t.Error("restartda parol tekshiruvi ishlashi kerak")
	}
}
