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
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chzyer/readline"
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
)

const basePrompt = "\033[1;32m❯\033[0m "

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

// ---- Spinner: readline prompt'ini animatsiya qiladi (satr tahririga xalaqit bermaydi) ----
type spinner struct {
	rl   *readline.Instance
	mu   sync.Mutex
	stop chan struct{}
	on   bool
}

var spFrame = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func (s *spinner) begin(label string) {
	s.mu.Lock()
	if s.on {
		close(s.stop)
		s.on = false
	}
	s.on = true
	s.stop = make(chan struct{})
	stop := s.stop
	s.mu.Unlock()
	go func() {
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.rl.SetPrompt(fmt.Sprintf("%c %s ", spFrame[i%len(spFrame)], label))
				s.rl.Refresh()
				i++
			}
		}
	}()
}

func (s *spinner) end() {
	s.mu.Lock()
	if s.on {
		close(s.stop)
		s.on = false
	}
	s.mu.Unlock()
	s.rl.SetPrompt(basePrompt)
	s.rl.Refresh()
}

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

var pulseArt = []string{
	` ██████╗ ██╗   ██╗██╗     ███████╗███████╗`,
	` ██╔══██╗██║   ██║██║     ██╔════╝██╔════╝`,
	` ██████╔╝██║   ██║██║     ███████╗█████╗  `,
	` ██╔═══╝ ██║   ██║██║     ╚════██║██╔══╝  `,
	` ██║     ╚██████╔╝███████╗███████║███████╗`,
	` ╚═╝      ╚═════╝ ╚══════╝╚══════╝╚══════╝`,
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
	for _, l := range pulseArt {
		fmt.Println(cGreenB + l + cReset)
	}
	fmt.Printf("\n %s❯_ ParkPulse DevOps agenti%s\n", cDim, cReset)
	fmt.Printf(" %sprovayder%s %s   %smodel%s %s\n", cDim, cReset, prov, cDim, cReset, model)
	fmt.Printf(" %s/new yangi sessiya · /stop to'xtatish · Ctrl+C to'xtatish/chiqish%s\n\n", cDim, cReset)
}

func printTool(w io.Writer, ev event) {
	if ev.State == "running" {
		arg := strings.TrimSpace(ev.Input)
		fmt.Fprintf(w, "\n%s⏺%s %s%s%s %s%s%s\n", cGreen, cReset, cBold, ev.Name, cReset, cDim, arg, cReset)
		return
	}
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
	for i, ln := range lines {
		pre := "   "
		if i == 0 {
			pre = " ⎿ "
		}
		fmt.Fprintf(w, "%s%s%s%s\n", col, pre, ln, cReset)
	}
	if clipped {
		fmt.Fprintf(w, "%s   … (chiqish qisqartirildi)%s\n", cDim, cReset)
	}
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

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            basePrompt,
		HistoryLimit:      300,
		InterruptPrompt:   "^C",
		EOFPrompt:         "",
		HistorySearchFold: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "terminal xatosi: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()
	w := rl.Stdout() // async chiqish shu yerga — prompt avtomatik qayta chiziladi
	sp := &spinner{rl: rl}

	// Yagona yozuvchi (gorilla WS bitta yozuvchi talab qiladi).
	var wmu sync.Mutex
	send := func(v any) {
		wmu.Lock()
		conn.WriteJSON(v)
		wmu.Unlock()
	}

	var running atomic.Bool // serverda vazifa ketyaptimi
	var mu sync.Mutex
	pending := "" // kutilayotgan tasdiq id'si

	go func() {
		for {
			var ev event
			if err := conn.ReadJSON(&ev); err != nil {
				sp.end()
				fmt.Fprint(w, "\n"+cDim+"[ulanish uzildi]"+cReset+"\n")
				rl.Close()
				os.Exit(0)
			}
			switch ev.Type {
			case "assistant":
				sp.end()
				fmt.Fprint(w, "\n"+ev.Text+"\n")
			case "tool":
				sp.end()
				printTool(w, ev)
				if ev.State == "running" {
					sp.begin(ev.Name + " bajarilyapti…")
				}
			case "confirm":
				sp.end()
				mu.Lock()
				pending = ev.ID
				mu.Unlock()
				fmt.Fprintf(w, "\n%s⚠  Xavfli buyruq:%s %s\n%s   sabab: %s%s\n",
					cYellow, cReset, ev.Command, cDim, ev.Reason, cReset)
				rl.SetPrompt(cYellow + "bajarilsinmi? [y/N]" + cReset + " ")
				rl.Refresh()
			case "status":
				switch ev.State {
				case "thinking":
					running.Store(true)
					sp.begin("o'ylayapti…")
				case "error":
					sp.end()
					msg := ev.Text
					if h := hint(msg); h != "" {
						msg += "\n" + cDim + "   → " + h + cReset
					}
					fmt.Fprintf(w, "\n%s✗ xato:%s %s\n", cRed, cReset, msg)
					running.Store(false)
				case "idle":
					sp.end()
					running.Store(false)
				case "reset":
					sp.end()
					running.Store(false)
					mu.Lock()
					pending = ""
					mu.Unlock()
					rl.SetPrompt(basePrompt)
					fmt.Fprint(w, "\n"+cGreen+"✎ yangi sessiya — tarix tozalandi"+cReset+"\n")
				case "busy":
					sp.end()
					fmt.Fprint(w, "\n"+cDim+"· avvalgi so'rov tugashini kuting (yoki /stop)"+cReset+"\n")
				}
			}
		}
	}()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt { // Ctrl+C
			if running.Load() {
				send(map[string]any{"type": "stop"})
				fmt.Fprint(w, cYellow+"■ to'xtatildi"+cReset+cDim+"  (yana Ctrl+C — chiqish · /new — yangi sessiya)"+cReset+"\n")
				continue
			}
			break
		} else if err == io.EOF { // Ctrl+D
			break
		}
		line = strings.TrimSpace(line)

		switch line {
		case "/new", "/reset", "/clear":
			mu.Lock()
			pending = ""
			mu.Unlock()
			rl.SetPrompt(basePrompt)
			send(map[string]any{"type": "new"})
			continue
		case "/stop":
			send(map[string]any{"type": "stop"})
			continue
		case "/help", "/?":
			fmt.Fprint(w, cDim+"\n  /new    yangi sessiya (tarixni tozalaydi)\n"+
				"  /stop   joriy vazifani to'xtatish (Ctrl+C ham)\n"+
				"  Ctrl+C  to'xtatish · yana bosilsa chiqish\n"+cReset)
			continue
		}

		mu.Lock()
		id := pending
		mu.Unlock()
		if id != "" { // tasdiq javobi
			approve := line == "y" || line == "Y" || line == "yes" || line == "ha"
			send(map[string]any{"type": "decision", "id": id, "approve": approve})
			mu.Lock()
			pending = ""
			mu.Unlock()
			rl.SetPrompt(basePrompt)
			if !approve {
				fmt.Fprint(w, cDim+"· rad etildi"+cReset+"\n")
			}
			continue
		}
		if line != "" {
			send(map[string]any{"type": "chat", "text": line})
		}
	}

	sp.end()
	fmt.Fprint(w, "\n"+cDim+"xayr 👋"+cReset+"\n")
}
