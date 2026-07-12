// Package alert kuzatuv oqimidagi muhim hodisalarni Telegram va/yoki webhook
// orqali ogohlantirishga aylantiradi. Grafana bo'lmasa ham ishlaydi.
//
// Kuzatiladigan hodisalar:
//   - qurilma uzildi / qayta tiklandi (netmon holat o'zgarishi bo'yicha)
//   - shubhali shlagbaum ochilishi (arvoh yoki qoidabuzarlik)
//   - SNMP interfeys uzildi / tiklandi
//
// Yuborish tashqi tarmoqqa boradi va bloklashi mumkin — shuning uchun hodisa
// kuzatish (Observe) tez ishlaydi va yuborish alohida goroutine'da navbat orqali
// bajariladi. Xabarlar holat o'zgarishida chiqadi (spam bo'lmasin).
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"parkpulse/backend/internal/analyzer"
	"parkpulse/backend/internal/netmon"
	"parkpulse/backend/internal/snmp"
)

// Notification — yuboriladigan bitta xabar.
type Notification struct {
	Level string    `json:"level"` // "critical" | "warning" | "ok"
	Title string    `json:"title"`
	Text  string    `json:"text"`
	Time  time.Time `json:"time"`
}

// Settings — UI/API va disk uchun sozlamalar (token'lar shu yerda saqlanadi).
type Settings struct {
	TelegramToken string `json:"telegram_token"`
	TelegramChat  string `json:"telegram_chat"`
	Webhook       string `json:"webhook"`
}

// Manager hodisalarni kuzatadi va sozlangan sinklarga xabar yuboradi.
// Sozlamalar runtime'da (UI orqali) o'zgarishi mumkin — shuning uchun mutex.
type Manager struct {
	mu      sync.RWMutex
	tgToken string
	tgChat  string
	webhook string

	store  string // sozlama fayli yo'li (JSON), UI o'zgartirsa yoziladi
	client *http.Client

	queue chan Notification

	// Holat o'zgarishini kuzatish (faqat Observe goroutine'idan, locksiz).
	devUp   map[string]bool // kalit: IP -> oxirgi tirik holati
	ifUp    map[string]bool // kalit: host/if -> oxirgi holati
	primed  bool            // birinchi snapshot'ni "boshlang'ich" deb qabul qilamiz
	ifKnown bool
}

// New sozlamalarni env'dan o'qib Manager qaytaradi. Keyin LoadStore() disk
// faylidagi (UI orqali kiritilgan) qiymatlarni ustiga qo'yadi.
//
//	ALERT_TELEGRAM_TOKEN, ALERT_TELEGRAM_CHAT — Telegram bot
//	ALERT_WEBHOOK_URL                         — ixtiyoriy JSON webhook
//	ALERT_STORE                               — sozlama fayli (standart: alerts.json)
func New() *Manager {
	store := strings.TrimSpace(os.Getenv("ALERT_STORE"))
	if store == "" {
		store = "alerts.json"
	}
	return &Manager{
		tgToken: strings.TrimSpace(os.Getenv("ALERT_TELEGRAM_TOKEN")),
		tgChat:  strings.TrimSpace(os.Getenv("ALERT_TELEGRAM_CHAT")),
		webhook: strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL")),
		store:   store,
		client:  &http.Client{Timeout: 10 * time.Second},
		queue:   make(chan Notification, 64),
		devUp:   make(map[string]bool),
		ifUp:    make(map[string]bool),
	}
}

// LoadStore disk faylidagi sozlamani (agar bo'lsa) yuklaydi. UI orqali kiritilgan
// qiymat env'dan ustun turadi (u eng so'nggi foydalanuvchi harakati).
func (m *Manager) LoadStore() {
	b, err := os.ReadFile(m.store)
	if err != nil {
		return // fayl yo'q — normal
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		log.Printf("[alert] sozlama fayli buzuq (%s): %v", m.store, err)
		return
	}
	m.mu.Lock()
	m.tgToken, m.tgChat, m.webhook = s.TelegramToken, s.TelegramChat, s.Webhook
	m.mu.Unlock()
}

// Config joriy sozlamani qaytaradi (UI ko'rsatishi uchun).
func (m *Manager) Config() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Settings{TelegramToken: m.tgToken, TelegramChat: m.tgChat, Webhook: m.webhook}
}

// SetConfig sozlamani yangilaydi va diskka yozadi (restartda saqlanadi).
func (m *Manager) SetConfig(s Settings) error {
	m.mu.Lock()
	m.tgToken = strings.TrimSpace(s.TelegramToken)
	m.tgChat = strings.TrimSpace(s.TelegramChat)
	m.webhook = strings.TrimSpace(s.Webhook)
	snap := Settings{TelegramToken: m.tgToken, TelegramChat: m.tgChat, Webhook: m.webhook}
	m.mu.Unlock()

	b, _ := json.MarshalIndent(snap, "", "  ")
	if err := os.WriteFile(m.store, b, 0o600); err != nil {
		log.Printf("[alert] sozlamani saqlab bo'lmadi (%s): %v", m.store, err)
		return err
	}
	return nil
}

// Test joriy sozlama bilan sinov xabarini YUBORADI (natijani darhol qaytaradi).
func (m *Manager) Test(ctx context.Context) error {
	if !m.Enabled() {
		return errors.New("hech qaysi kanal sozlanmagan")
	}
	n := Notification{Level: "ok", Title: "✅ ParkPulse test", Text: "Ogohlantirish ishlayapti", Time: time.Now()}
	tg, wh := m.snapshot()
	var errs []string
	if tg.token != "" && tg.chat != "" {
		if err := m.sendTelegram(ctx, n); err != nil {
			errs = append(errs, "Telegram: "+err.Error())
		}
	}
	if wh != "" {
		if err := m.sendWebhook(ctx, n); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

type tgConf struct{ token, chat string }

// snapshot config maydonlarini lock ostida nusxalaydi (yuborish paytida band bo'lmasin).
func (m *Manager) snapshot() (tgConf, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return tgConf{token: m.tgToken, chat: m.tgChat}, m.webhook
}

// Enabled — kamida bitta sink sozlanganmi?
func (m *Manager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return (m.tgToken != "" && m.tgChat != "") || m.webhook != ""
}

// Sinks — sozlangan sinklarning inson o'qiy oladigan ro'yxati (log uchun).
func (m *Manager) Sinks() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var s []string
	if m.tgToken != "" && m.tgChat != "" {
		s = append(s, "Telegram")
	}
	if m.webhook != "" {
		s = append(s, "webhook")
	}
	return s
}

// Run yuborish worker'ini boshlaydi (navbatni ketma-ket qayta ishlaydi).
func (m *Manager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-m.queue:
			m.deliver(ctx, n)
		}
	}
}

// Observe kuzatuv oqimidagi bitta xabarni ko'radi va kerak bo'lsa ogohlantiradi.
// Tez ishlaydi — haqiqiy yuborish worker'da bo'ladi.
func (m *Manager) Observe(msg analyzer.Message) {
	switch d := msg.Data.(type) {
	case analyzer.OpenEvent:
		m.onOpen(d)
	case []netmon.Device:
		m.onDevices(d)
	case []snmp.Host:
		m.onSNMP(d)
	}
}

func (m *Manager) onOpen(o analyzer.OpenEvent) {
	if !o.Kind.Suspicious() {
		return
	}
	title, level := "⚠️ Qoidabuzarlik ochilish", "warning"
	if o.Kind == analyzer.KindGhost {
		title, level = "🚨 Arvoh ochilish", "critical"
	}
	where := o.Gate
	if o.Plate != "" {
		where = o.Plate + " · " + o.Gate
	}
	m.enqueue(Notification{Level: level, Title: title, Text: where + " — " + o.Reason})
}

func (m *Manager) onDevices(devs []netmon.Device) {
	for _, d := range devs {
		// Faqat kuzatiladigan qurilmalar (kamera/relay/POS) xabar beradi.
		// Skanerda topilgan telefon/noutbuk uzilib-ulanaversa bezovta qilmaydi.
		if !d.Watched {
			continue
		}
		prev, seen := m.devUp[d.IP]
		m.devUp[d.IP] = d.Alive
		if !m.primed || !seen || prev == d.Alive {
			continue // birinchi ko'rish yoki o'zgarish yo'q — xabar yo'q
		}
		name := d.Name
		if name == "" {
			name = d.IP
		}
		if d.Alive {
			m.enqueue(Notification{Level: "ok", Title: "🟢 Qurilma tiklandi", Text: name + " (" + d.IP + ")"})
		} else {
			m.enqueue(Notification{Level: "critical", Title: "🔴 Qurilma uzildi", Text: name + " (" + d.IP + ")"})
		}
	}
	m.primed = true
}

func (m *Manager) onSNMP(hosts []snmp.Host) {
	for _, h := range hosts {
		for _, i := range h.Ifaces {
			key := h.IP + "/" + i.Name
			prev, seen := m.ifUp[key]
			m.ifUp[key] = i.Up
			if !m.ifKnown || !seen || prev == i.Up {
				continue
			}
			where := h.Name + " · " + i.Name
			if i.Up {
				m.enqueue(Notification{Level: "ok", Title: "🟢 Port tiklandi", Text: where})
			} else {
				m.enqueue(Notification{Level: "warning", Title: "🟠 Port uzildi", Text: where})
			}
		}
	}
	m.ifKnown = true
}

func (m *Manager) enqueue(n Notification) {
	n.Time = time.Now()
	select {
	case m.queue <- n:
	default:
		log.Printf("[alert] navbat to'la — xabar tashlab yuborildi: %s", n.Title)
	}
}

func (m *Manager) deliver(ctx context.Context, n Notification) {
	tg, wh := m.snapshot()
	if tg.token != "" && tg.chat != "" {
		if err := m.sendTelegram(ctx, n); err != nil {
			log.Printf("[alert] telegram: %v", err)
		}
	}
	if wh != "" {
		if err := m.sendWebhook(ctx, n); err != nil {
			log.Printf("[alert] webhook: %v", err)
		}
	}
}

func (m *Manager) sendTelegram(ctx context.Context, n Notification) error {
	tg, _ := m.snapshot()
	text := "*" + tgEscape(n.Title) + "*\n" + tgEscape(n.Text) +
		"\n_" + n.Time.Format("15:04:05") + "_"
	form := url.Values{}
	form.Set("chat_id", tg.chat)
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")
	endpoint := "https://api.telegram.org/bot" + tg.token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) sendWebhook(ctx context.Context, n Notification) error {
	_, wh := m.snapshot()
	body, _ := json.Marshal(n)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// tgEscape Telegram Markdown'da maxsus belgilarni himoyalaydi.
func tgEscape(s string) string {
	r := strings.NewReplacer("_", " ", "*", " ", "`", "'", "[", "(", "]", ")")
	return r.Replace(s)
}
