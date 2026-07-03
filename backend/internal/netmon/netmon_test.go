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
