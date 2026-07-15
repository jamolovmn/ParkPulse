package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"parkpulse/backend/internal/agent"
	"parkpulse/backend/internal/alert"
	"parkpulse/backend/internal/analyzer"
	"parkpulse/backend/internal/collector"
	"parkpulse/backend/internal/config"
	"parkpulse/backend/internal/detector"
	"parkpulse/backend/internal/logbuf"
	"parkpulse/backend/internal/netmon"
	"parkpulse/backend/internal/snmp"
	"parkpulse/backend/internal/speedtest"
	"parkpulse/backend/internal/ws"
)

func main() {
	// Ixtiyoriy YAML konfig env'ni to'ldiradi (aniq env har doim ustun).
	if path, err := config.Load(); err != nil {
		log.Fatalf("konfig: %v", err)
	} else if path != "" {
		log.Printf("konfig o'qildi: %s", path)
	}

	// Konteyner nomi env orqali kelishi mumkin: TARGET_CONTAINER="p24gui"
	// (bir nechta bo'lsa vergul bilan). Endi bu MAJBURIY EMAS — UI'dan ham
	// tanlanadi. Bo'sh bo'lsa, saqlangan tanlov yoki UI kutiladi.
	var names []string
	for _, n := range strings.Split(os.Getenv("TARGET_CONTAINER"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8888"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	col, err := collector.New(names)
	if err != nil {
		log.Fatalf("docker client: %v", err)
	}
	buf := logbuf.New(30) // arvoh konteksti: oxirgi 30 qator
	col.Buf = buf
	det := detector.New() // aqlli log o'qish: shablon + to'lov korrelyatsiyasi
	col.Detector = det
	ai := agent.New() // AI DevOps yordamchisi (kalit kiritilmaguncha uxlaydi)
	ai.LoadStore()
	aiHub := agent.NewHub(ai) // CLI + Web bir xil sessiyaga ulanadi
	anl := analyzer.New()
	anl.ContextFn = buf.Snapshot
	mon := netmon.New()
	hub := ws.NewHub()

	col.Run(ctx)
	go anl.Run(ctx, col.Events)
	go mon.Run(ctx)

	// Analyzer va netmon xabarlarini bitta oqimga yig'ib hub'ga beramiz
	msgs := make(chan analyzer.Message, 512)
	go pipe(ctx, anl.Out, msgs)
	go pipe(ctx, mon.Out, msgs)

	// Sog'liq monitoridan kelgan xabarlarni ham hub'ga yuboramiz
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case h := <-col.HealthOut:
				msgs <- analyzer.Message{Type: "health", Data: h}
			}
		}
	}()

	// SNMP poller (switch/router interfeyslari) — SNMP_TARGETS berilgan bo'lsa.
	if pol := snmp.New(); pol != nil {
		go pol.Run(ctx)
		go pipe(ctx, pol.Out, msgs)
		log.Printf("SNMP kuzatuvi: %d ta qurilma", pol.Targets())
	}

	go runSpeedtest(ctx, msgs) // internet tezligini davriy o'lchaydi

	// Ogohlantirish har doim ishlaydi (UI orqali runtime'da yoqilishi mumkin).
	// Barcha xabarlar hub'ga borishdan oldin alerter'dan o'tadi.
	al := alert.New()
	al.LoadStore() // UI orqali kiritilgan sozlama (agar bo'lsa)
	go al.Run(ctx)
	hubIn := make(chan analyzer.Message, 512)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-msgs:
				al.Observe(m)
				hubIn <- m
			}
		}
	}()
	go hub.Run(ctx, hubIn)
	if al.Enabled() {
		log.Printf("ogohlantirish yoqilgan: %v", al.Sinks())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Prometheus eksporti — Grafana/Prometheus'ga ulash uchun.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		hub.WriteMetrics(w)
	})
	// Ogohlantirish sozlamalari — UI'dan Telegram token/chat va webhook kiritiladi.
	mux.HandleFunc("/api/alerts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"config":  al.Config(),
				"enabled": al.Enabled(),
				"sinks":   al.Sinks(),
			})
		case http.MethodPost:
			var s alert.Settings
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				http.Error(w, "yaroqsiz JSON", http.StatusBadRequest)
				return
			}
			if err := al.SetConfig(s); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"enabled": al.Enabled(), "sinks": al.Sinks()})
		default:
			http.Error(w, "GET yoki POST kerak", http.StatusMethodNotAllowed)
		}
	})
	// Sinov xabari — sozlama to'g'riligini tekshirish uchun.
	mux.HandleFunc("/api/alerts/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		tctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := al.Test(tctx); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	// Qurilma kuzatuvini yoqish/o'chirish — faqat kuzatiladiganlar alert beradi.
	mux.HandleFunc("/api/devices/watch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			IP      string `json:"ip"`
			Watched bool   `json:"watched"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IP == "" {
			http.Error(w, "ip kerak", http.StatusBadRequest)
			return
		}
		mon.SetWatch(body.IP, body.Watched)
		w.WriteHeader(http.StatusNoContent)
	})

	// Qurilmaga qo'lda nom berish (bo'sh nom — avtomatik nomga qaytaradi).
	mux.HandleFunc("/api/devices/name", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			IP   string `json:"ip"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IP == "" {
			http.Error(w, "ip kerak", http.StatusBadRequest)
			return
		}
		mon.SetName(body.IP, body.Name)
		w.WriteHeader(http.StatusNoContent)
	})

	// Agent so'rovidan tokenni oladi (header yoki ?token=).
	agentToken := func(r *http.Request) string {
		if t := r.Header.Get("X-Pulse-Token"); t != "" {
			return t
		}
		return r.URL.Query().Get("token")
	}

	// Parol bilan kirish — to'g'ri bo'lsa qurilma saqlaydigan token qaytaradi.
	mux.HandleFunc("/api/agent/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		tok, ok := ai.Login(body.Password)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"ok": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "token": tok})
	})

	// AI agent jonli oqimi (WS) — parol yoqilgan bo'lsa token talab qiladi.
	mux.HandleFunc("/api/agent/stream", func(w http.ResponseWriter, r *http.Request) {
		if ai.AuthEnabled() && !ai.ValidToken(agentToken(r)) {
			http.Error(w, "parol kerak", http.StatusUnauthorized)
			return
		}
		aiHub.HandleWS(w, r)
	})

	// AI agent sozlamasi — GET holat (ochiq); POST — parol yoqilgan bo'lsa token kerak.
	mux.HandleFunc("/api/agent/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ai.Status())
		case http.MethodPost:
			if ai.AuthEnabled() && !ai.ValidToken(agentToken(r)) {
				http.Error(w, "parol kerak", http.StatusUnauthorized)
				return
			}
			var s agent.Settings
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				http.Error(w, "yaroqsiz JSON", http.StatusBadRequest)
				return
			}
			if err := ai.SetConfig(s); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ai.Status())
		default:
			http.Error(w, "GET yoki POST kerak", http.StatusMethodNotAllowed)
		}
	})
	// Provayderdagi modellar ro'yxati — UI'da avto tanlash uchun.
	mux.HandleFunc("/api/agent/models", func(w http.ResponseWriter, r *http.Request) {
		if ai.AuthEnabled() && !ai.ValidToken(agentToken(r)) {
			http.Error(w, "parol kerak", http.StatusUnauthorized)
			return
		}
		tctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		models, err := ai.Models(tctx)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"models": []string{}, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"models": models})
	})

	// Kalitni tekshirish — provayderga minimal so'rov.
	mux.HandleFunc("/api/agent/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		tctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := ai.Test(tctx); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	// Log inspector — oxirgi qatorlar + yorliq va avtomatik o'rganilgan shablonlar.
	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"lines":   det.Lines(),
			"learned": det.Learned(),
		})
	})

	// Docker konteynerlar ro'yxati — UI'da kuzatiladiganini tanlash uchun.
	mux.HandleFunc("/api/containers", func(w http.ResponseWriter, r *http.Request) {
		list, err := col.ListContainers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"containers": list})
	})
	// Kuzatiladigan konteynerlarni o'qish/o'rnatish (loglari real vaqtda tail qilinadi).
	mux.HandleFunc("/api/target", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"targets": col.Targets()})
		case http.MethodPost:
			var body struct {
				Targets []string `json:"targets"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "yaroqsiz JSON", http.StatusBadRequest)
				return
			}
			col.SetTargets(body.Targets)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"targets": col.Targets()})
		default:
			http.Error(w, "GET yoki POST kerak", http.StatusMethodNotAllowed)
		}
	})

	// Subnet skaner — tarmoqdagi qurilmalarni topadi
	mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST kerak", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Subnet string `json:"subnet"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		devs, err := mon.Scan(r.Context(), body.Subnet)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"found": len(devs), "devices": devs})
	})

	// All-in-One: shu binary Next.js static build'ini ham serve qiladi.
	mux.Handle("/", staticHandler(staticDir()))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	log.Printf("ParkPulse ishga tushdi. Konteynerlar: %v, UI+WS: http://localhost%s", names, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func pipe(ctx context.Context, in <-chan analyzer.Message, out chan<- analyzer.Message) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-in:
			out <- m
		}
	}
}

// runSpeedtest internet tezligini davriy o'lchaydi va WS orqali yuboradi.
// Nechta brauzer ochiq bo'lsa ham server bitta test qiladi — natija umumiy.
// SPEEDTEST_MIN=0 bilan o'chiriladi; standart 15 daqiqa.
func runSpeedtest(ctx context.Context, out chan<- analyzer.Message) {
	mins := 15
	if v, err := strconv.Atoi(os.Getenv("SPEEDTEST_MIN")); err == nil {
		mins = v
	}
	if mins <= 0 {
		return
	}
	run := func() {
		res, err := speedtest.Run(ctx)
		if err != nil {
			log.Printf("[speedtest] %v", err)
			return
		}
		select {
		case out <- analyzer.Message{Type: "speedtest", Data: res}:
		case <-ctx.Done():
		}
	}
	run() // ishga tushganda darhol bir marta
	t := time.NewTicker(time.Duration(mins) * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

func staticDir() string {
	if d := os.Getenv("STATIC_DIR"); d != "" {
		return d
	}
	return "./static"
}

// staticHandler Next.js export'ini serve qiladi; topilmagan yo'llarga index.html
// qaytaradi (SPA fallback). Papka bo'lmasa (dev rejim) faqat ogohlantiradi.
func staticHandler(dir string) http.Handler {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		log.Printf("[static] %s topilmadi — UI'siz, faqat /ws rejimida ishlayapman", dir)
	}
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
