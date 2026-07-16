package agent

import "testing"

// Reset suhbat tarixini tozalashi va reset hodisasini yuborishi kerak.
func TestResetClearsHistory(t *testing.T) {
	h := NewHub(New())
	h.hist = append(h.hist, turn{role: "user", text: "salom"})

	sub := make(chan Event, 8)
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()

	h.Reset()

	if len(h.hist) != 0 {
		t.Fatalf("tarix tozalanmadi: %d ta qoldi", len(h.hist))
	}
	sawReset := false
	for len(sub) > 0 {
		if (<-sub).State == "reset" {
			sawReset = true
		}
	}
	if !sawReset {
		t.Fatal("reset hodisasi yuborilmadi")
	}
}

// Stop hech qanday vazifa ketmayotganda ham panik bermasligi kerak (cancel == nil).
func TestStopNilCancel(t *testing.T) {
	NewHub(New()).Stop()
}
