package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"parkpulse/backend/internal/analyzer"
	"parkpulse/backend/internal/collector"
	"parkpulse/backend/internal/logbuf"
	"parkpulse/backend/internal/ws"
)

func main() {
	// Har obyektda konteyner nomi har xil (p24gui, parking24-gateway-api, ...).
	// Shuning uchun nom faqat env orqali keladi: TARGET_CONTAINER="p24gui"
	// Bir nechta bo'lsa vergul bilan: TARGET_CONTAINER="p24gui,p24-relay"
	names := strings.Split(os.Getenv("TARGET_CONTAINER"), ",")
	if names[0] == "" {
		log.Fatal("TARGET_CONTAINER env o'rnatilmagan (masalan: TARGET_CONTAINER=p24gui)")
	}
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
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
	anl := analyzer.New()
	anl.ContextFn = buf.Snapshot
	hub := ws.NewHub()

	col.Run(ctx)
	go anl.Run(ctx, col.Events)
	go hub.Run(ctx, anl.Out)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.HandleWS)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
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
