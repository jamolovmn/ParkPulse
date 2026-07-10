package ws

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteMetrics hub'ning joriy holatini Prometheus text-exposition formatida
// yozadi. Bu odamlarga ParkPulse'ni Grafana/Prometheus'ga ulash imkonini beradi
// — tashqi kutubxonasiz, oddiy matn.
func (h *Hub) WriteMetrics(w io.Writer) {
	h.mu.Lock()
	s := h.state // sayoz nusxa; ichidagi slice/map'lar Run'da o'rniga qo'yiladi, joyida o'zgarmaydi
	h.mu.Unlock()

	// --- Kirish/o'tish statistikasi ---
	help(w, "parkpulse_passes_total", "counter", "Jami muvaffaqiyatli o'tishlar (ANPR -> to'lov).")
	metric(w, "parkpulse_passes_total", nil, float64(s.Stats.TotalPasses))

	help(w, "parkpulse_avg_latency_ms", "gauge", "O'rtacha ANPR->to'lov latencysi (avto-to'lovlarsiz), ms.")
	metric(w, "parkpulse_avg_latency_ms", nil, s.Stats.AvgLatencyMs)

	help(w, "parkpulse_ghost_openings_total", "gauge", "Shubhali ochilishlar (qoidabuzarlik + arvoh).")
	metric(w, "parkpulse_ghost_openings_total", nil, float64(s.Stats.GhostCount))

	help(w, "parkpulse_opens_total", "gauge", "Shlagbaum ochilishlari, turi bo'yicha.")
	for _, kind := range sortedKeys(s.Stats.Opens) {
		metric(w, "parkpulse_opens_total", labels{"kind": kind}, float64(s.Stats.Opens[kind]))
	}

	// --- Qurilma sifati ---
	help(w, "parkpulse_device_up", "gauge", "Qurilma javob berayaptimi (1/0).")
	help(w, "parkpulse_device_rtt_ms", "gauge", "Qurilmaning oxirgi RTT'si, ms.")
	help(w, "parkpulse_device_jitter_ms", "gauge", "Qurilma RTT jitteri (ketma-ket farq o'rtachasi), ms.")
	help(w, "parkpulse_device_loss_ratio", "gauge", "Paket yo'qotish ulushi (0..1).")
	help(w, "parkpulse_device_uptime_ratio", "gauge", "Oynadagi javob berish ulushi (0..1).")
	for _, d := range s.Devices {
		lb := labels{"ip": d.IP, "name": d.Name}
		metric(w, "parkpulse_device_up", lb, b2f(d.Alive))
		metric(w, "parkpulse_device_rtt_ms", lb, d.RttMs)
		metric(w, "parkpulse_device_jitter_ms", lb, d.JitterMs)
		metric(w, "parkpulse_device_loss_ratio", lb, d.LossPct/100)
		metric(w, "parkpulse_device_uptime_ratio", lb, d.UptimePct/100)
	}

	// --- Server sog'ligi ---
	if s.Health != nil {
		help(w, "parkpulse_cpu_percent", "gauge", "Yadro bandligi, foiz.")
		for i, c := range s.Health.Cores {
			metric(w, "parkpulse_cpu_percent", labels{"core": fmt.Sprintf("%d", i)}, c)
		}
		help(w, "parkpulse_ram_used_mb", "gauge", "Ishlatilgan xotira, MB.")
		metric(w, "parkpulse_ram_used_mb", nil, s.Health.UsedRAM)
		help(w, "parkpulse_ram_total_mb", "gauge", "Jami xotira, MB.")
		metric(w, "parkpulse_ram_total_mb", nil, s.Health.TotalRAM)
		help(w, "parkpulse_uptime_seconds", "gauge", "Server ishlash vaqti, soniya.")
		metric(w, "parkpulse_uptime_seconds", nil, s.Health.UptimeSec)

		help(w, "parkpulse_container_cpu_percent", "gauge", "Konteyner CPU bandligi, foiz.")
		help(w, "parkpulse_container_ram_mb", "gauge", "Konteyner xotirasi, MB.")
		for _, c := range s.Health.Containers {
			lb := labels{"name": c.Name}
			metric(w, "parkpulse_container_cpu_percent", lb, c.CPU)
			metric(w, "parkpulse_container_ram_mb", lb, c.RAM_MB)
		}
	}

	// --- Internet tezligi ---
	if s.Speed != nil {
		help(w, "parkpulse_speedtest_download_mbps", "gauge", "Yuklab olish tezligi, Mbit/s.")
		metric(w, "parkpulse_speedtest_download_mbps", nil, s.Speed.DownloadMbps)
		help(w, "parkpulse_speedtest_upload_mbps", "gauge", "Yuklash tezligi, Mbit/s.")
		metric(w, "parkpulse_speedtest_upload_mbps", nil, s.Speed.UploadMbps)
		help(w, "parkpulse_speedtest_ping_ms", "gauge", "Internet ping, ms.")
		metric(w, "parkpulse_speedtest_ping_ms", nil, s.Speed.PingMs)
	}
}

type labels map[string]string

func help(w io.Writer, name, typ, text string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, text, name, typ)
}

func metric(w io.Writer, name string, lb labels, val float64) {
	if len(lb) == 0 {
		fmt.Fprintf(w, "%s %s\n", name, ftoa(val))
		return
	}
	keys := make([]string, 0, len(lb))
	for k := range lb {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, escapeLabel(lb[k])))
	}
	fmt.Fprintf(w, "%s{%s} %s\n", name, strings.Join(parts, ","), ftoa(val))
}

func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

func ftoa(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", v), "0"), ".")
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
