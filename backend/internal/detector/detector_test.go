package detector

import (
	"fmt"
	"testing"
	"time"

	"parkpulse/backend/internal/parser"
)

func TestTemplatize(t *testing.T) {
	a := templatize("Relay exit 1: opened at 12:00:05")
	b := templatize("Relay exit 27: opened at 09:33:41")
	if a != b {
		t.Errorf("bir xil format bitta shablon bo'lishi kerak:\n a=%q\n b=%q", a, b)
	}
	if templatize("Relay exit 1: opened") == templatize("POS payment done") {
		t.Error("boshqa qatorlar boshqa shablon bo'lishi kerak")
	}
}

// Detektor to'lovdan keyin muntazam kelgan tanilmagan qatorni "ochilish" deb
// o'rganib, keyin undan sintetik OPEN yasashi kerak (yo'nalish = chiqish).
func TestLearnsOpenFromPaymentCorrelation(t *testing.T) {
	d := New()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	pos := func(ts time.Time) *parser.Event {
		return &parser.Event{Type: parser.EventPOS, Timestamp: ts, Container: "c", Gate: "exit 1"}
	}

	var synthetic *parser.Event
	for i := 0; i < 8; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		// To'lov (tanilgan hodisa).
		d.Feed("c", "Vendotek chiqish 1: Requesting payment", ts, "POS", pos(ts))
		// Undan ~2s keyin — tanilmagan jismoniy ochilish qatori (raqami har xil).
		openLine := fmt.Sprintf("Rele chiqdi %d: signal yuborildi", 100+i)
		out := d.Feed("c", openLine, ts.Add(2*time.Second), "", nil)
		for _, e := range out {
			if e.Type == parser.EventOpen {
				synthetic = e
			}
		}
	}

	if synthetic == nil {
		t.Fatal("korrelyatsiyadan keyin sintetik OPEN kutilgan edi, chiqmadi")
	}
	if synthetic.Gate != "exit 1" {
		t.Errorf("to'lov ergashgani uchun 'exit 1' kutildi, %q chiqdi", synthetic.Gate)
	}
	if len(d.Learned()) == 0 {
		t.Error("o'rganilgan shablon ro'yxati bo'sh bo'lmasligi kerak")
	}
}

// O'rganilgandan keyin, faqat ANPR bo'lgan (to'lovsiz) ochilish -> kirish.
func TestSyntheticEntryWhenNoPayment(t *testing.T) {
	d := New()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	pos := func(ts time.Time) *parser.Event {
		return &parser.Event{Type: parser.EventPOS, Timestamp: ts, Container: "c", Gate: "exit 1"}
	}
	// Shablonni o'rgatamiz (to'lov + ochilish).
	for i := 0; i < 8; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		d.Feed("c", "Vendotek chiqish 1: Requesting payment", ts, "POS", pos(ts))
		d.Feed("c", fmt.Sprintf("Rele chiqdi %d: signal", 100+i), ts.Add(2*time.Second), "", nil)
	}
	// Endi: ANPR, keyin o'sha ochilish shabloni, to'lovsiz -> kirish.
	t2 := base.Add(time.Hour)
	d.Feed("c", "-------- 01A777BC --------", t2, "ANPR",
		&parser.Event{Type: parser.EventANPR, Timestamp: t2, Container: "c", Plate: "01A777BC"})
	out := d.Feed("c", "Rele chiqdi 999: signal", t2.Add(time.Second), "", nil)

	var gate string
	for _, e := range out {
		if e.Type == parser.EventOpen {
			gate = e.Gate
		}
	}
	if gate != "enter 1" {
		t.Errorf("to'lovsiz, ANPR ergashgan ochilish 'enter 1' bo'lishi kerak, %q chiqdi", gate)
	}
}
