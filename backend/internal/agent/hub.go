package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// hub.go — bitta agent sessiyasi. CLI va Web bir xil hub'ga ulanadi, shuning uchun
// har hodisa (o'ylash, tool ishga tushishi, tasdiq so'rovi) ikkalasiga ham boradi —
// holat sinxron. Bir vaqtda bitta suhbat qadami ishlaydi (busy qulf).

const (
	maxSteps      = 30 // bitta xabarga tool-tsikl qadamlari chegarasi (chuqur tekshirish uchun)
	confirmExpiry = 5 * time.Minute
)

const baseSystemPrompt = `You are the ParkPulse DevOps agent — an autonomous operator
with REAL shell access on this server. You diagnose and FIX operational issues
(crashed containers, bad configs, resource problems) end-to-end, by yourself.

Tools: bash, read_file, write_file, docker_ps, docker_logs, docker_restart.
Privileged commands work: sudo is installed and a sudo password may be configured,
so "sudo <cmd>" runs non-interactively. Never ask the user to run a command — run
it yourself with the bash tool.

HOW TO WORK — this is the most important part:
- NEVER guess and NEVER give shallow answers. "It probably restarted" is a failure.
  Find the ROOT cause with concrete evidence: WHY did it restart? OOM kill? panic?
  non-zero exit code? a specific error line in the logs? a failed dependency?
- Investigate iteratively, like a senior engineer: form a hypothesis → gather
  evidence with tools → refine → repeat. Chain MANY tool calls before concluding.
- Don't assume the environment. Detect it first when unsure: check the OS
  (cat /etc/os-release), which tools exist, container names (docker ps). Use the
  right commands: docker logs --tail, docker inspect (State.ExitCode, OOMKilled,
  RestartCount), journalctl, dmesg | grep -i oom, df -h, free -m, ps aux, exit codes.
- Read the ACTUAL logs, don't skim. grep for error/fatal/panic/killed, look at
  timestamps around the incident, follow the chain of events.
- When you fix something: make the MINIMAL change, then VERIFY it worked (re-run the
  check, confirm the service is healthy). Never claim success without proof.
- Final answer must be specific: the root cause, the evidence you found, what you
  changed, and how you verified it. Be concise but concrete. Reply in the user's
  language.
- Destructive commands (rm, drop, kill/stop containers, mkfs, ...) are gated: call
  them normally — the harness pauses and asks the user for Y/N before running them.`

// systemPrompt — asosiy ko'rsatma + operator bergan loyiha ko'rsatmasi (AGENT.md).
// Har so'rovda qayta o'qiladi, shuning uchun faylni tahrirlash darhol ta'sir qiladi.
func systemPrompt() string {
	p := baseSystemPrompt
	if extra := readInstructions(); extra != "" {
		p += "\n\n# Deployment-specific notes (from the operator — trust these)\n" + extra
	}
	return p
}

// readInstructions loyihaga xos ko'rsatmani o'qiydi. Yo'l: AGENT_INSTRUCTIONS env
// yoki standart "AGENT.md". Bu yerga operator o'z tizimi haqida yozadi: konteyner
// nomlari, log formati, tez-tez uchraydigan muammolar, qadamlar va h.k.
func readInstructions() string {
	path := strings.TrimSpace(os.Getenv("AGENT_INSTRUCTIONS"))
	if path == "" {
		path = "AGENT.md"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const max = 16000
	if len(b) > max {
		b = b[:max]
	}
	return strings.TrimSpace(string(b))
}

// Event — CLI va Web'ga yuboriladigan yagona hodisa konverti.
type Event struct {
	Type    string `json:"type"`              // status | assistant | log | tool | confirm
	State   string `json:"state,omitempty"`   // status/tool holati
	Text    string `json:"text,omitempty"`    // assistant/log matni
	ID      string `json:"id,omitempty"`      // tool/confirm identifikatori
	Name    string `json:"name,omitempty"`    // tool nomi
	Input   string `json:"input,omitempty"`   // tool argumenti
	Output  string `json:"output,omitempty"`  // tool natijasi
	Exit    int    `json:"exit,omitempty"`    // tool chiqish kodi
	Command string `json:"command,omitempty"` // confirm: buyruq
	Reason  string `json:"reason,omitempty"`  // confirm: nega xavfli
}

type Hub struct {
	mgr *Manager
	reg *Registry

	mu      sync.Mutex
	subs    map[chan Event]struct{}
	pending map[string]chan bool
	hist    []turn
	busy    bool
	cancel  context.CancelFunc // ishlayotgan vazifani bekor qilish uchun
}

func NewHub(mgr *Manager) *Hub {
	return &Hub{
		mgr:     mgr,
		reg:     NewRegistry(mgr),
		subs:    map[chan Event]struct{}{},
		pending: map[string]chan bool{},
	}
}

func (h *Hub) emit(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Chat yangi foydalanuvchi xabarini qabul qiladi va tool-tsiklni goroutine'da yuritadi.
func (h *Hub) Chat(text string) {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		h.emit(Event{Type: "status", State: "busy", Text: "avvalgi so'rov tugashini kuting"})
		return
	}
	if !h.mgr.Enabled() {
		h.mu.Unlock()
		h.emit(Event{Type: "status", State: "error", Text: "AI kalit kiritilmagan — Tizim → AI agent"})
		return
	}
	h.busy = true
	h.hist = append(h.hist, turn{role: "user", text: text})
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			h.busy = false
			h.cancel = nil
			h.mu.Unlock()
			cancel()
		}()
		h.run(ctx)
	}()
}

// Stop ishlayotgan vazifani bekor qiladi (tarix saqlanadi). Kutayotgan tasdiqlar
// ham rad etilgan deb yopiladi. CLI/Web'dagi Ctrl+C yoki "stop" shu yerga keladi.
func (h *Hub) Stop() {
	h.mu.Lock()
	c := h.cancel
	for _, ch := range h.pending {
		select {
		case ch <- false:
		default:
		}
	}
	h.mu.Unlock()
	if c != nil {
		c()
	}
	h.emit(Event{Type: "status", State: "idle"})
}

// Reset sessiyani butunlay tozalaydi: vazifani to'xtatadi va suhbat tarixini
// o'chiradi — keyingi so'rov toza kontekstdan boshlanadi.
func (h *Hub) Reset() {
	h.Stop()
	h.mu.Lock()
	h.hist = nil
	h.mu.Unlock()
	h.emit(Event{Type: "status", State: "reset"})
}

// Decide tasdiq javobini kutayotgan tool'ga uzatadi (CLI yoki Web'dan).
func (h *Hub) Decide(id string, approve bool) {
	h.mu.Lock()
	ch := h.pending[id]
	h.mu.Unlock()
	if ch != nil {
		select {
		case ch <- approve:
		default:
		}
	}
}

func (h *Hub) run(ctx context.Context) {
	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil { // to'xtatildi
			h.emit(Event{Type: "status", State: "idle"})
			return
		}
		h.emit(Event{Type: "status", State: "thinking"})
		text, calls, err := h.mgr.complete(ctx, systemPrompt(), h.snapshotHist(), h.reg.Specs())
		if err != nil {
			if ctx.Err() != nil { // xato emas — foydalanuvchi to'xtatdi
				h.emit(Event{Type: "status", State: "idle"})
				return
			}
			h.emit(Event{Type: "status", State: "error", Text: err.Error()})
			return
		}
		h.mu.Lock()
		h.hist = append(h.hist, turn{role: "assistant", text: text, toolCalls: calls})
		h.mu.Unlock()

		if text != "" {
			h.emit(Event{Type: "assistant", Text: text})
		}
		if len(calls) == 0 {
			h.emit(Event{Type: "status", State: "idle"})
			return
		}

		for _, c := range calls {
			if ctx.Err() != nil {
				h.emit(Event{Type: "status", State: "idle"})
				return
			}
			arg := guardArg(c.name, c.args)
			h.emit(Event{Type: "tool", State: "running", ID: c.id, Name: c.name, Input: arg})

			if dstr, reason := destructive(c.name, arg); dstr {
				if !h.confirm(ctx, c.id, describe(c.name, arg), reason) {
					h.appendTool(c.id, "foydalanuvchi rad etdi")
					h.emit(Event{Type: "tool", State: "done", ID: c.id, Name: c.name, Output: "rad etildi", Exit: 1})
					continue
				}
			}

			out, rerr := h.reg.Run(ctx, c.name, c.args)
			exit := 0
			if rerr != nil {
				out += "\nXATO: " + rerr.Error()
				exit = 1
			}
			h.appendTool(c.id, out)
			h.emit(Event{Type: "tool", State: "done", ID: c.id, Name: c.name, Output: out, Exit: exit})
		}
	}
	h.emit(Event{Type: "status", State: "idle"})
}

func (h *Hub) snapshotHist() []turn {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]turn, len(h.hist))
	copy(out, h.hist)
	return out
}

func (h *Hub) appendTool(id, text string) {
	h.mu.Lock()
	h.hist = append(h.hist, turn{role: "tool", toolCallID: id, text: text})
	h.mu.Unlock()
}

// confirm xavfli tool uchun Y/N so'raydi va CLI/Web javobini kutadi.
func (h *Hub) confirm(ctx context.Context, id, command, reason string) bool {
	ch := make(chan bool, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	h.emit(Event{Type: "confirm", ID: id, Command: command, Reason: reason})
	h.emit(Event{Type: "status", State: "waiting"})
	defer func() { h.mu.Lock(); delete(h.pending, id); h.mu.Unlock() }()
	select {
	case ok := <-ch:
		return ok
	case <-ctx.Done(): // to'xtatildi
		return false
	case <-time.After(confirmExpiry):
		return false
	}
}

func describe(name, arg string) string {
	if name == "bash" {
		return arg
	}
	return fmt.Sprintf("%s %s", name, arg)
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// HandleWS — GET /api/agent/stream. CLI ham shu endpointga ulanadi.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	enabled := h.mgr.Enabled()
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.subs, ch); close(ch); h.mu.Unlock() }()

	go func() {
		for ev := range ch {
			if conn.WriteJSON(ev) != nil {
				return
			}
		}
	}()

	state := "idle"
	if !enabled {
		state = "error"
	}
	conn.WriteJSON(Event{Type: "status", State: state})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			ID      string `json:"id"`
			Approve bool   `json:"approve"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "chat":
			h.Chat(msg.Text)
		case "decision":
			h.Decide(msg.ID, msg.Approve)
		case "stop":
			h.Stop()
		case "new", "reset":
			h.Reset()
		}
	}
}
