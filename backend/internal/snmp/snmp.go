// Package snmp sozlangan tarmoq qurilmalarini (switch, router) SNMP orqali
// davriy so'rab turadi: har interfeysning holati (up/down), nomi va real vaqtdagi
// trafigi (kirish/chiqish Mbps). Natija WS orqali dashboard'ga va /metrics'ga chiqadi.
//
// Trafik oktet hisoblagichlaridan olinadi: ikki o'lchov orasidagi farq vaqtga
// bo'linadi. Shuning uchun birinchi so'rovda Mbps ko'rinmaydi (asos o'lchov).
//
// Sozlash (env yoki YAML):
//
//	SNMP_TARGETS="Core=192.168.1.1@public,Edge=192.168.1.2@public"
//	SNMP_INTERVAL_SEC=30
package snmp

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"parkpulse/backend/internal/analyzer"
)

// Standart SNMP MIB-II OID'lari.
const (
	oidSysDescr     = "1.3.6.1.2.1.1.1.0"
	oidSysUpTime    = "1.3.6.1.2.1.1.3.0"
	oidIfDescr      = "1.3.6.1.2.1.2.2.1.2"     // ifTable nomi (fallback)
	oidIfSpeed      = "1.3.6.1.2.1.2.2.1.5"     // bps (32-bit)
	oidIfOperStatus = "1.3.6.1.2.1.2.2.1.8"     // 1=up
	oidIfInOctets   = "1.3.6.1.2.1.2.2.1.10"    // 32-bit (fallback)
	oidIfOutOctets  = "1.3.6.1.2.1.2.2.1.16"    // 32-bit (fallback)
	oidIfName       = "1.3.6.1.2.1.31.1.1.1.1"  // ifXTable qisqa nomi
	oidIfHCIn       = "1.3.6.1.2.1.31.1.1.1.6"  // 64-bit kirish oktetlari
	oidIfHCOut      = "1.3.6.1.2.1.31.1.1.1.10" // 64-bit chiqish oktetlari
	oidIfHighSpeed  = "1.3.6.1.2.1.31.1.1.1.15" // Mbps
)

const (
	defaultInterval = 30 * time.Second
	snmpTimeout     = 2 * time.Second
	maxIfaces       = 128 // juda katta switch'da UI to'lib ketmasin
)

// Iface — bitta tarmoq interfeysining holati.
type Iface struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"`
	Up        bool    `json:"up"`
	InMbps    float64 `json:"in_mbps"`
	OutMbps   float64 `json:"out_mbps"`
	SpeedMbps float64 `json:"speed_mbps,omitempty"` // liniya tezligi (nominal)
}

// Host — bitta SNMP qurilmasi va uning interfeyslari.
type Host struct {
	Name   string  `json:"name"`
	IP     string  `json:"ip"`
	Up     bool    `json:"up"` // SNMP javob berdimi
	Descr  string  `json:"descr,omitempty"`
	Uptime string  `json:"uptime,omitempty"`
	Ifaces []Iface `json:"ifaces"`
	Err    string  `json:"err,omitempty"`
}

// Target — poll qilinadigan qurilma.
type Target struct {
	Name      string
	IP        string
	Community string
	Version   string // "1" yoki "2c" (standart: 2c)
}

type counter struct {
	in, out uint64
	t       time.Time
}

type Poller struct {
	Out chan analyzer.Message

	targets  []Target
	interval time.Duration
	mu       sync.Mutex
	prev     map[string]counter // kalit: ip/index
}

// New env'dan targetlarni o'qiydi. Target bo'lmasa nil qaytaradi (poller o'chirilgan).
func New() *Poller {
	targets := parseTargets(os.Getenv("SNMP_TARGETS"))
	if len(targets) == 0 {
		return nil
	}
	interval := defaultInterval
	if v, err := strconv.Atoi(os.Getenv("SNMP_INTERVAL_SEC")); err == nil && v > 0 {
		interval = time.Duration(v) * time.Second
	}
	return &Poller{
		Out:      make(chan analyzer.Message, 8),
		targets:  targets,
		interval: interval,
		prev:     make(map[string]counter),
	}
}

// parseTargets "Core=1.1.1.1@public,Edge=2.2.2.2" ni Target ro'yxatiga aylantiradi.
func parseTargets(s string) []Target {
	var out []Target
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, rest := part, part
		if n, r, ok := strings.Cut(part, "="); ok {
			name, rest = strings.TrimSpace(n), strings.TrimSpace(r)
		}
		ip, community := rest, "public"
		if h, c, ok := strings.Cut(rest, "@"); ok {
			ip, community = strings.TrimSpace(h), strings.TrimSpace(c)
		}
		// Ixtiyoriy versiya: "public#1" -> SNMP v1 (eski qurilmalar uchun).
		version := "2c"
		if c, v, ok := strings.Cut(community, "#"); ok {
			community, version = strings.TrimSpace(c), strings.TrimSpace(v)
		}
		if ip == "" {
			continue
		}
		if name == rest || name == "" {
			name = ip
		}
		out = append(out, Target{Name: name, IP: ip, Community: community, Version: version})
	}
	return out
}

// Targets tashqi sozlamadan (config) targetlarni o'rnatadi — env bo'sh bo'lsa main chaqiradi.
func (p *Poller) Targets() int { return len(p.targets) }

func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.pollAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.pollAll(ctx)
		}
	}
}

func (p *Poller) pollAll(ctx context.Context) {
	hosts := make([]Host, len(p.targets))
	var wg sync.WaitGroup
	for i, tg := range p.targets {
		wg.Add(1)
		go func(i int, tg Target) {
			defer wg.Done()
			hosts[i] = p.poll(ctx, tg)
		}(i, tg)
	}
	wg.Wait()
	select {
	case p.Out <- analyzer.Message{Type: "snmp", Data: hosts}:
	default:
	}
}

func (p *Poller) poll(ctx context.Context, tg Target) Host {
	h := Host{Name: tg.Name, IP: tg.IP}
	g := &gosnmp.GoSNMP{
		Target:    tg.IP,
		Port:      161,
		Community: tg.Community,
		Version:   gosnmp.Version2c,
		Timeout:   snmpTimeout,
		Retries:   1,
		Context:   ctx,
	}
	if tg.Version == "1" {
		g.Version = gosnmp.Version1
	}
	if err := g.Connect(); err != nil {
		h.Err = "ulanmadi: " + err.Error()
		return h
	}
	defer g.Conn.Close()

	// Skalarlar: sysDescr, sysUpTime
	if res, err := g.Get([]string{oidSysDescr, oidSysUpTime}); err == nil {
		for _, v := range res.Variables {
			switch {
			case strings.HasPrefix(v.Name, "."+oidSysDescr), v.Name == oidSysDescr:
				h.Descr = firstLine(toStr(v.Value))
			case strings.HasPrefix(v.Name, "."+oidSysUpTime), v.Name == oidSysUpTime:
				h.Uptime = formatTicks(toUint(v.Value))
			}
		}
	} else {
		h.Err = "javob yo'q: " + err.Error()
		return h
	}
	h.Up = true

	names := walkStr(g, oidIfName)
	if len(names) == 0 {
		names = walkStr(g, oidIfDescr)
	}
	status := walkUint(g, oidIfOperStatus)
	inHC := walkUint(g, oidIfHCIn)
	if len(inHC) == 0 {
		inHC = walkUint(g, oidIfInOctets)
	}
	outHC := walkUint(g, oidIfHCOut)
	if len(outHC) == 0 {
		outHC = walkUint(g, oidIfOutOctets)
	}
	speed := walkUint(g, oidIfHighSpeed) // Mbps
	speedLow := walkUint(g, oidIfSpeed)  // bps

	now := time.Now()
	idxs := sortedKeys(status)
	for _, idx := range idxs {
		if len(h.Ifaces) >= maxIfaces {
			break
		}
		name := names[idx]
		if name == "" {
			name = "if" + strconv.Itoa(idx)
		}
		iface := Iface{Index: idx, Name: name, Up: status[idx] == 1}
		if v, ok := speed[idx]; ok && v > 0 {
			iface.SpeedMbps = float64(v)
		} else if v, ok := speedLow[idx]; ok && v > 0 {
			iface.SpeedMbps = float64(v) / 1e6
		}

		key := tg.IP + "/" + strconv.Itoa(idx)
		in, out := inHC[idx], outHC[idx]
		p.mu.Lock()
		if prev, ok := p.prev[key]; ok {
			dt := now.Sub(prev.t).Seconds()
			if dt > 0 {
				iface.InMbps = bps(prev.in, in, dt)
				iface.OutMbps = bps(prev.out, out, dt)
			}
		}
		p.prev[key] = counter{in: in, out: out, t: now}
		p.mu.Unlock()

		h.Ifaces = append(h.Ifaces, iface)
	}
	return h
}

// bps oktet farqidan Mbps hisoblaydi. Hisoblagich qayta boshlansa (manfiy farq)
// yoki restartda — 0 qaytaramiz (noto'g'ri sakrash ko'rsatmaslik uchun).
func bps(prev, cur uint64, dt float64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur-prev) * 8 / dt / 1e6
}

func walkStr(g *gosnmp.GoSNMP, oid string) map[int]string {
	out := make(map[int]string)
	_ = g.BulkWalk(oid, func(pdu gosnmp.SnmpPDU) error {
		if idx, ok := lastIndex(pdu.Name); ok {
			out[idx] = firstLine(toStr(pdu.Value))
		}
		return nil
	})
	return out
}

func walkUint(g *gosnmp.GoSNMP, oid string) map[int]uint64 {
	out := make(map[int]uint64)
	_ = g.BulkWalk(oid, func(pdu gosnmp.SnmpPDU) error {
		if idx, ok := lastIndex(pdu.Name); ok {
			out[idx] = toUint(pdu.Value)
		}
		return nil
	})
	return out
}

func lastIndex(oid string) (int, bool) {
	if i := strings.LastIndex(oid, "."); i >= 0 {
		if n, err := strconv.Atoi(oid[i+1:]); err == nil {
			return n, true
		}
	}
	return 0, false
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func toUint(v any) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case uint:
		return uint64(x)
	case uint32:
		return uint64(x)
	case int:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	default:
		return 0
	}
}

// formatTicks SNMP TimeTicks (1/100 s) ni "12d 3h 45m" ko'rinishiga o'giradi.
func formatTicks(ticks uint64) string {
	sec := ticks / 100
	d := sec / 86400
	hh := (sec % 86400) / 3600
	mm := (sec % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, hh, mm)
	case hh > 0:
		return fmt.Sprintf("%dh %dm", hh, mm)
	default:
		return fmt.Sprintf("%dm", mm)
	}
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func sortedKeys(m map[int]uint64) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
