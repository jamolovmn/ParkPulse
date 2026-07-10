package netmon

import (
	"context"
	"net"
	"strconv"
	"testing"
)

// Ochiq TCP porti bo'lgan qurilma — ping bo'lmasa ham tirik deb topilishi kerak.
func TestTcpPingOpenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	_, alive := tcpPing(context.Background(), "127.0.0.1", []int{port})
	if !alive {
		t.Error("ochiq portli qurilma tirik deb topilmadi")
	}
}

// Port yopiq, lekin qurilma javob beradi (connection refused) — tirik.
func TestTcpPingRefused(t *testing.T) {
	// Band bo'lmagan portni topamiz va darhol yopamiz
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // endi bu portga ulanish "refused" beradi

	_, alive := tcpPing(context.Background(), "127.0.0.1", []int{port})
	if !alive {
		t.Errorf("refused javobi tirik deb hisoblanishi kerak (port %s)", strconv.Itoa(port))
	}
}

// Yo'q qurilma (TEST-NET, marshrutlanmaydi) — o'lik.
func TestTcpPingDead(t *testing.T) {
	_, alive := tcpPing(context.Background(), "192.0.2.1", []int{80, 443})
	if alive {
		t.Error("marshrutlanmaydigan IP tirik deb topildi")
	}
}

func TestRecordQuality(t *testing.T) {
	d := &Device{}
	// 8 ta javob (10,12,10,14,10,12,10,12) + 2 ta yo'qotish
	rtts := []float64{10, 12, 10, 14, 10, 12, 10, 12}
	for _, r := range rtts {
		record(d, true, r)
	}
	record(d, false, 0)
	record(d, false, 0)

	if d.MinMs != 10 || d.MaxMs != 14 {
		t.Errorf("min/max = %v/%v, kutilgan 10/14", d.MinMs, d.MaxMs)
	}
	// 10 sampladan 2 tasi yo'qolgan -> 20% loss, 80% uptime
	if d.LossPct != 20 || d.UptimePct != 80 {
		t.Errorf("loss/uptime = %v/%v, kutilgan 20/80", d.LossPct, d.UptimePct)
	}
	if d.JitterMs <= 0 {
		t.Errorf("jitter musbat bo'lishi kerak edi, olindi %v", d.JitterMs)
	}
	// Sparkline oxiridagi ikki nuqta yo'qotish (-1) bo'lishi kerak
	n := len(d.Samples)
	if n < 2 || d.Samples[n-1] != -1 || d.Samples[n-2] != -1 {
		t.Errorf("sparkline oxirida -1 kutilgan edi: %v", d.Samples)
	}
}

// Oyna histLen dan oshsa, eski namunalar tushib qolishi kerak.
func TestRecordWindowTrim(t *testing.T) {
	d := &Device{}
	for i := 0; i < histLen+15; i++ {
		record(d, true, 5)
	}
	if len(d.hist) != histLen {
		t.Errorf("tarix uzunligi = %d, kutilgan %d", len(d.hist), histLen)
	}
	if d.LossPct != 0 || d.UptimePct != 100 {
		t.Errorf("to'liq tirik: loss/uptime = %v/%v", d.LossPct, d.UptimePct)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		ports  []int
		vendor string
		want   string
	}{
		{[]int{80, 554}, "", "Kamera"},
		{[]int{37777}, "", "Kamera"},
		{[]int{80}, "Hikvision", "Kamera"},
		{[]int{80}, "", "Web qurilma"},
		{[]int{9999}, "", "Ochiq portli qurilma"},
		{nil, "", "Noma'lum"},
	}
	for _, c := range cases {
		if got := classify(c.ports, c.vendor); got != c.want {
			t.Errorf("classify(%v,%q) = %q, kutilgan %q", c.ports, c.vendor, got, c.want)
		}
	}
}
