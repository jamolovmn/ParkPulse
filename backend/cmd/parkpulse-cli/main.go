// Command pulse-cli — ParkPulse AI agentiga terminal interfeysi (claude code kabi).
//
// Serverning /api/agent/stream WS endpointiga ulanadi — Web UI bilan AYNAN bitta
// agent sessiyasi. Bir joyda boshlangan ish ikkinchisida ham ko'rinadi.
//
//	pulse-cli                      # ws://localhost:8888 ga ulanadi
//	pulse-cli ws://host:8888       # boshqa manzil
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---- ANSI ranglar ----
const (
	cReset  = "\033[0m"
	cDim    = "\033[90m"
	cGreen  = "\033[32m"
	cGreenB = "\033[1;32m"
	cCyan   = "\033[36m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cBold   = "\033[1m"
	clr     = "\r\033[K" // qatorni tozalash
)

type statusInfo struct {
	Auth     bool   `json:"auth"`
	Enabled  bool   `json:"enabled"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

func fetchStatus(httpBase string) (statusInfo, error) {
	var st statusInfo
	resp, err := http.Get(httpBase + "/api/agent/config")
	if err != nil {
		return st, err
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(&st)
	return st, nil
}

// resolveToken parol yoqilgan bo'lsa tokenni oladi (PULSE_PASSWORD env yoki so'rov).
func resolveToken(httpBase string, st statusInfo) (string, error) {
	if !st.Auth {
		return "", nil
	}
	pw := os.Getenv("PULSE_PASSWORD")
	if pw == "" {
		fmt.Printf("%sAgent paroli:%s ", cYellow, cReset)
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		pw = strings.TrimSpace(line)
	}
	body, _ := json.Marshal(map[string]string{"password": pw})
	lr, err := http.Post(httpBase+"/api/agent/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer lr.Body.Close()
	if lr.StatusCode != http.StatusOK {
		return "", fmt.Errorf("parol noto'g'ri")
	}
	var d struct {
		Token string `json:"token"`
	}
	json.NewDecoder(lr.Body).Decode(&d)
	return d.Token, nil
}

func httpFromWS(u string) string {
	h := strings.Replace(u, "wss://", "https://", 1)
	h = strings.Replace(h, "ws://", "http://", 1)
	return strings.TrimSuffix(h, "/api/agent/stream")
}

type event struct {
	Type    string `json:"type"`
	State   string `json:"state"`
	Text    string `json:"text"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Input   string `json:"input"`
	Output  string `json:"output"`
	Exit    int    `json:"exit"`
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

// ---- Spinner (o'ylayapti/bajarilyapti indikatori) ----
var (
	outMu   sync.Mutex // stdout'ni serializatsiya qiladi
	spStop  chan struct{}
	spDone  chan struct{}
	spOn    bool
	spMu    sync.Mutex
	spFrame = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
)

func startSpin(label string) {
	spMu.Lock()
	defer spMu.Unlock()
	if spOn {
		return
	}
	spOn = true
	spStop = make(chan struct{})
	spDone = make(chan struct{})
	go func(stop, done chan struct{}) {
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-stop:
				outMu.Lock()
				fmt.Print(clr)
				outMu.Unlock()
				close(done)
				return
			case <-t.C:
				outMu.Lock()
				fmt.Printf("\r%s%c %s%s", cDim, spFrame[i%len(spFrame)], label, cReset)
				outMu.Unlock()
				i++
			}
		}
	}(spStop, spDone)
}

func stopSpin() {
	spMu.Lock()
	on := spOn
	stop, done := spStop, spDone
	spOn = false
	spMu.Unlock()
	if on {
		close(stop)
		<-done // qator tozalanguncha kutamiz
	}
}

func out(s string) {
	outMu.Lock()
	fmt.Print(s)
	outMu.Unlock()
}

func prompt() { out("\n" + cGreenB + "❯ " + cReset) }

// hint — tarmoq/timeout xatolariga foydali maslahat qo'shadi.
func hint(errText string) string {
	e := strings.ToLower(errText)
	switch {
	case strings.Contains(e, "deadline exceeded"), strings.Contains(e, "timeout"):
		return "provayder javob bermadi (tarmoq sekin/bloklangan yoki model juda sekin). " +
			"Boshqa provayderni sinang yoki AGENT_TIMEOUT_SEC ni oshiring."
	case strings.Contains(e, "no such host"), strings.Contains(e, "connection refused"), strings.Contains(e, "dial tcp"):
		return "provayder manziliga ulanib bo'lmadi — serverdan internet/DNS ochiqligini tekshiring."
	case strings.Contains(e, "401"), strings.Contains(e, "403"), strings.Contains(e, "unauthorized"):
		return "API kalit noto'g'ri yoki ruxsat yo'q — Agent sozlamalarida kalitni tekshiring."
	case strings.Contains(e, "429"):
		return "limit oshib ketdi (rate limit) — biroz kuting yoki boshqa model."
	}
	return ""
}

func banner(st statusInfo) {
	model := st.Model
	if model == "" {
		model = "(model tanlanmagan)"
	}
	prov := st.Provider
	if prov == "" {
		prov = "—"
	}
	fmt.Println()
	fmt.Println(cGreenB + "  ❯_ Pulse" + cReset + cDim + "   ParkPulse DevOps agenti" + cReset)
	fmt.Println(cDim + "  ─────────────────────────────────────────────" + cReset)
	fmt.Printf("  %sprovayder%s %s   %smodel%s %s\n", cDim, cReset, prov, cDim, cReset, model)
	fmt.Println(cDim + "  Savol yozing. Xavfli buyruqlar tasdiq so'raydi. Chiqish: Ctrl+D" + cReset)
}

func printTool(ev event) {
	if ev.State == "running" {
		arg := strings.TrimSpace(ev.Input)
		out(fmt.Sprintf("\n%s⏺%s %s%s%s %s%s%s\n", cGreen, cReset, cBold, ev.Name, cReset, cDim, arg, cReset))
		return
	}
	// done — chiqishni ⎿ bilan chekilgan holda ko'rsatamiz (claude uslubi)
	body := strings.TrimRight(ev.Output, "\n")
	col := cDim
	if ev.Exit != 0 {
		col = cRed
	}
	lines := strings.Split(body, "\n")
	const maxLines = 40
	clipped := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		clipped = true
	}
	var b strings.Builder
	for i, ln := range lines {
		pre := "   "
		if i == 0 {
			pre = " ⎿ "
		}
		b.WriteString(fmt.Sprintf("%s%s%s%s\n", col, pre, ln, cReset))
	}
	if clipped {
		b.WriteString(fmt.Sprintf("%s   … (chiqish qisqartirildi)%s\n", cDim, cReset))
	}
	out(b.String())
}

func main() {
	url := "ws://localhost:8888/api/agent/stream"
	if len(os.Args) > 1 {
		url = strings.TrimSuffix(os.Args[1], "/")
		if !strings.Contains(url, "/api/agent/stream") {
			url += "/api/agent/stream"
		}
	}
	httpBase := httpFromWS(url)

	st, err := fetchStatus(httpBase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sserverga ulanib bo'lmadi (%s): %v%s\n", cRed, httpBase, err, cReset)
		os.Exit(1)
	}
	if !st.Enabled {
		fmt.Fprintf(os.Stderr, "%sAgent hali sozlanmagan — Web UI → Agent bo'limida API kalit kiriting.%s\n", cYellow, cReset)
		os.Exit(1)
	}
	if tok, err := resolveToken(httpBase, st); err != nil {
		fmt.Fprintf(os.Stderr, "%skirish xatosi: %v%s\n", cRed, err, cReset)
		os.Exit(1)
	} else if tok != "" {
		url += "?token=" + tok
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sulanib bo'lmadi (%s): %v%s\n", cRed, url, err, cReset)
		os.Exit(1)
	}
	defer conn.Close()

	banner(st)

	var mu sync.Mutex
	pending := "" // kutilayotgan tasdiq id'si
	active := false

	go func() {
		for {
			var ev event
			if err := conn.ReadJSON(&ev); err != nil {
				stopSpin()
				out("\n" + cDim + "[ulanish uzildi]" + cReset + "\n")
				os.Exit(0)
			}
			switch ev.Type {
			case "assistant":
				stopSpin()
				out("\n" + ev.Text + "\n")
			case "tool":
				stopSpin()
				printTool(ev)
				if ev.State == "running" {
					startSpin(ev.Name + " bajarilyapti…")
				}
			case "confirm":
				stopSpin()
				mu.Lock()
				pending = ev.ID
				mu.Unlock()
				out(fmt.Sprintf("\n%s⚠  Xavfli buyruq:%s %s\n%s   sabab: %s%s\n%s   Bajarilsinmi? [y/N]:%s ",
					cYellow, cReset, ev.Command, cDim, ev.Reason, cReset, cBold, cReset))
			case "status":
				switch ev.State {
				case "thinking":
					active = true
					startSpin("o'ylayapti…")
				case "error":
					stopSpin()
					msg := ev.Text
					if h := hint(msg); h != "" {
						msg += "\n" + cDim + "   → " + h + cReset
					}
					out(fmt.Sprintf("\n%s✗ xato:%s %s\n", cRed, cReset, msg))
					if active {
						active = false
						prompt()
					}
				case "idle":
					stopSpin()
					if active {
						active = false
						prompt()
					}
				case "busy":
					stopSpin()
					out("\n" + cDim + "· avvalgi so'rov tugashini kuting…" + cReset + "\n")
				}
			}
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // uzun so'rovlar uchun
	prompt()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		mu.Lock()
		id := pending
		mu.Unlock()
		if id != "" { // tasdiq javobi
			approve := line == "y" || line == "Y" || line == "yes" || line == "ha"
			conn.WriteJSON(map[string]any{"type": "decision", "id": id, "approve": approve})
			mu.Lock()
			pending = ""
			mu.Unlock()
			if !approve {
				out(cDim + "· rad etildi" + cReset + "\n")
			}
		} else if line != "" {
			conn.WriteJSON(map[string]any{"type": "chat", "text": line})
		} else {
			prompt()
		}
	}
	// Ctrl+D
	stopSpin()
	out("\n" + cDim + "xayr 👋" + cReset + "\n")
}
