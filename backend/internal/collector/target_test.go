package collector

import (
	"path/filepath"
	"testing"
)

func TestTargetPersistence(t *testing.T) {
	store := filepath.Join(t.TempDir(), "target.json")
	t.Setenv("TARGET_STORE", store)

	c, err := New([]string{"env-default"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// UI'dan tanlangandek (root nil — tail boshlanmaydi, faqat saqlanadi).
	c.SetTargets([]string{"b", "a", " a ", ""})

	got := c.loadTargets()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("saqlangan tanlov saralangan [a b] bo'lishi kerak, chiqdi: %v", got)
	}

	// Restart: yangi collector fayldan o'qiydi.
	c2, err := New([]string{"env-default"})
	if err != nil {
		t.Fatalf("New2: %v", err)
	}
	if lt := c2.loadTargets(); len(lt) != 2 {
		t.Errorf("restartda tanlov tiklanmadi: %v", lt)
	}
}
