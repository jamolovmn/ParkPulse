// Package netmon tarmoqdagi qurilmalarni (kamera, relay, POS...) kuzatadi:
// har 10s da ping yuborib, holatini WS orqali dashboard'ga chiqaradi.
// Qurilmalar ro'yxati: DEVICES env ("kamera=192.168.1.64,relay=192.168.1.70")
// va/yoki subnet skaner (Scan) orqali avtomatik topiladi.
package netmon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"parkpulse/backend/internal/analyzer"
)

func insecureTLS() *tls.Config { return &tls.Config{InsecureSkipVerify: true} }

const (
	pingInterval = 10 * time.Second
	pingTimeout  = 2 * time.Second
	maxScanHosts = 1024
	scanParallel = 128
)

type Device struct {
	Name     string    `json:"name"`
	IP       string    `json:"ip"`
	Alive    bool      `json:"alive"`
	RttMs    float64   `json:"rtt_ms"`
	LastSeen time.Time `json:"last_seen,omitempty"` // oxirgi javob bergan vaqti
	Type     string    `json:"type,omitempty"`      // avto aniqlangan: Kamera/Web/Noma'lum
	Vendor   string    `json:"vendor,omitempty"`    // HTTP izidan (Hikvision, Dahua...)
	Ports    []int     `json:"ports,omitempty"`     // ochiq portlar
	probed   bool      // fingerprint bir marta bajarildi
}

type Monitor struct {
	Out chan analyzer.Message

	mu      sync.Mutex
	devices map[string]*Device // kalit: IP
}

func New() *Monitor {
	m := &Monitor{
		Out:     make(chan analyzer.Message, 16),
		devices: make(map[string]*Device),
	}
	for _, pair := range strings.Split(os.Getenv("DEVICES"), ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, ip, ok := strings.Cut(pair, "=")
		if !ok {
			name, ip = pair, pair
		}
		m.devices[strings.TrimSpace(ip)] = &Device{Name: strings.TrimSpace(name), IP: strings.TrimSpace(ip)}
	}
	return m
}

func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	m.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *Monitor) tick(ctx context.Context) {
	m.mu.Lock()
	ips := make([]string, 0, len(m.devices))
	for ip := range m.devices {
		ips = append(ips, ip)
	}
	m.mu.Unlock()
	if len(ips) == 0 {
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, scanParallel)
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			rtt, alive := ping(ctx, ip)
			var needFP bool
			m.mu.Lock()
			if d, ok := m.devices[ip]; ok {
				d.Alive, d.RttMs = alive, rtt
				if alive {
					d.LastSeen = time.Now()
					needFP = !d.probed
				}
			}
			m.mu.Unlock()
			// Tur avtomatik aniqlanadi — tirik va hali skanerlanmagan bo'lsa
			if needFP {
				typ, vendor, ports := fingerprint(ctx, ip)
				m.mu.Lock()
				if d, ok := m.devices[ip]; ok {
					d.Type, d.Vendor, d.Ports, d.probed = typ, vendor, ports, true
				}
				m.mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	m.emit()
}

// Fingerprint uchun tekshiriladigan portlar (qurilma turini bildiradi).
var probePorts = []int{80, 443, 554, 8000, 37777, 34567}

// HTTP javobidan ishlab chiqaruvchini taxmin qiluvchi izlar.
var vendorHints = map[string]*regexp.Regexp{
	"Hikvision":  regexp.MustCompile(`(?i)hikvision|app-webs|dvrdvs|web service`),
	"Dahua":      regexp.MustCompile(`(?i)dahua|webserver|/current_config`),
	"Axis":       regexp.MustCompile(`(?i)axis`),
	"Boa/IP-cam": regexp.MustCompile(`(?i)server:\s*boa`),
}

// fingerprint qurilmaning ochiq portlarini skanerlab, turini aniqlaydi.
// Bir marta bajariladi (natija saqlanadi) — ping tick'da qayta ishlamaydi.
func fingerprint(ctx context.Context, ip string) (typ, vendor string, ports []int) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, p := range probePorts {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			d := net.Dialer{Timeout: 500 * time.Millisecond}
			c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
			if err != nil {
				return
			}
			c.Close()
			mu.Lock()
			ports = append(ports, port)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	sort.Ints(ports)

	vendor = httpVendor(ctx, ip, ports)
	typ = classify(ports, vendor)
	return
}

func classify(ports []int, vendor string) string {
	has := func(p int) bool {
		for _, x := range ports {
			if x == p {
				return true
			}
		}
		return false
	}
	switch {
	case has(554) || has(37777) || has(34567) || has(8000):
		return "Kamera" // RTSP/ONVIF/Dahua/DVR portlari
	case vendor != "":
		return "Kamera" // HTTP izi kamera ishlab chiqaruvchisini ko'rsatdi
	case has(80) || has(443):
		return "Web qurilma"
	case len(ports) > 0:
		return "Ochiq portli qurilma"
	default:
		return "Noma'lum"
	}
}

// httpVendor 80/443 portidan javob olib, ishlab chiqaruvchini taxmin qiladi.
func httpVendor(ctx context.Context, ip string, ports []int) string {
	scheme := ""
	for _, p := range ports {
		if p == 80 {
			scheme = "http"
			break
		}
		if p == 443 {
			scheme = "https"
		}
	}
	if scheme == "" {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, scheme+"://"+ip+"/", nil)
	if err != nil {
		return ""
	}
	tr := &http.Transport{TLSClientConfig: insecureTLS(), DisableKeepAlives: true}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	blob := "server: " + resp.Header.Get("Server") + " " + resp.Header.Get("WWW-Authenticate") + " " + string(body)
	for name, re := range vendorHints {
		if re.MatchString(blob) {
			return name
		}
	}
	return ""
}

var reRtt = regexp.MustCompile(`time=([0-9.]+) ms`)

// ping bitta ICMP so'rov yuboradi (busybox/iputils ping, konteyner root'da ishlaydi).
func ping(ctx context.Context, ip string) (rttMs float64, alive bool) {
	cctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, "ping", "-c", "1", "-W", "1", ip).Output()
	if err != nil {
		return 0, false
	}
	if mm := reRtt.FindSubmatch(out); mm != nil {
		v, _ := strconv.ParseFloat(string(mm[1]), 64)
		return v, true
	}
	return 0, true
}

// Scan subnet'dagi tirik qurilmalarni topib ro'yxatga qo'shadi.
// subnet bo'sh bo'lsa: SCAN_SUBNET env, u ham bo'lmasa interfeyslardan aniqlanadi.
func (m *Monitor) Scan(ctx context.Context, subnet string) ([]Device, error) {
	var cidrs []string
	switch {
	case subnet != "":
		cidrs = []string{subnet}
	case os.Getenv("SCAN_SUBNET") != "":
		for _, c := range strings.Split(os.Getenv("SCAN_SUBNET"), ",") {
			cidrs = append(cidrs, strings.TrimSpace(c))
		}
	default:
		cidrs = localSubnets()
	}
	if len(cidrs) == 0 {
		return nil, errors.New("skanerlash uchun subnet topilmadi — konteynerga SCAN_SUBNET env bering (masalan 192.168.1.0/24) yoki ioEdge'da network mode: host qiling")
	}

	var ips []string
	for _, c := range cidrs {
		hosts, err := hostsOf(c)
		if err != nil {
			return nil, fmt.Errorf("subnet xato (%s): %w", c, err)
		}
		ips = append(ips, hosts...)
	}
	if len(ips) > maxScanHosts {
		return nil, fmt.Errorf("juda katta diapazon (%d ta IP) — /24 yoki kichikroq subnet bering", len(ips))
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, scanParallel)
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			if rtt, alive := ping(ctx, ip); alive {
				typ, vendor, ports := fingerprint(ctx, ip)
				m.mu.Lock()
				if _, ok := m.devices[ip]; !ok {
					m.devices[ip] = &Device{Name: ip, IP: ip}
				}
				d := m.devices[ip]
				d.Alive, d.RttMs, d.LastSeen = true, rtt, time.Now()
				d.Type, d.Vendor, d.Ports, d.probed = typ, vendor, ports, true
				m.mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	m.emit()
	return m.Devices(), nil
}

// Devices holatning saralangan nusxasini qaytaradi.
func (m *Monitor) Devices() []Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Monitor) emit() {
	select {
	case m.Out <- analyzer.Message{Type: "devices", Data: m.Devices()}:
	default:
	}
}

// localSubnets konteyner interfeyslaridan haqiqiy LAN subnetlarni oladi.
// Docker bridge (172.16/12) va loopback chiqarib tashlanadi — bular LAN emas.
// Host network rejimida bu ro'yxat serverning haqiqiy tarmoqlarini beradi.
func localSubnets() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 { // docker bridge
				continue
			}
			out = append(out, ipnet.String())
		}
	}
	return out
}

func hostsOf(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return nil, err
	}
	var out []string
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		out = append(out, ip.String())
		if len(out) > maxScanHosts+2 {
			break
		}
	}
	if len(out) > 2 { // tarmoq va broadcast manzillarini tashlab yuborish
		out = out[1 : len(out)-1]
	}
	return out, nil
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}
