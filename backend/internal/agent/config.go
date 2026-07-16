// Package agent — ParkPulse ichiga o'rnatilgan AI DevOps yordamchisi.
//
// config.go SOZLAMA + KIRISH (auth) qatlami: LLM provayderi, API kaliti, model
// va ixtiyoriy PAROL. Agent kalit kiritilmaguncha uxlaydi. Parol o'rnatilsa,
// jonli oqim (agent bash/docker ishlatadigan) va sozlamani o'zgartirish token
// talab qiladi — brauzer tokenni saqlaydi (o'sha qurilmada qayta so'ramaydi),
// boshqa qurilma parol so'raydi.
//
// Ko'p provayder: har biri OpenAI-mos (base_url + model) yoki Anthropic. NVIDIA,
// OpenRouter, MiniMax, DeepSeek va h.k. — hammasi base_url orqali qo'llanadi.
package agent

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Provider string

const (
	ProviderAnthropic  Provider = "anthropic"
	ProviderOpenAI     Provider = "openai"
	ProviderOpenRouter Provider = "openrouter"
	ProviderNvidia     Provider = "nvidia"
	ProviderLocal      Provider = "local"
)

// defaults — provayder bo'yicha standart baza URL va model. base_url har doim
// tahrirlanadi, shuning uchun ro'yxatda yo'q provayderni ham (MiniMax, Groq...)
// "openai" tanlab, base_url berib ishlatish mumkin.
var defaults = map[Provider]struct{ baseURL, model string }{
	ProviderAnthropic:  {"https://api.anthropic.com", "claude-opus-4-8"},
	ProviderOpenAI:     {"https://api.openai.com/v1", "gpt-4o"},
	ProviderOpenRouter: {"https://openrouter.ai/api/v1", "anthropic/claude-opus-4-8"},
	ProviderNvidia:     {"https://integrate.api.nvidia.com/v1", "meta/llama-3.1-70b-instruct"},
	ProviderLocal:      {"http://localhost:11434/v1", "llama3.1"},
}

// Settings — API/UI kirishi (kalit va parol faqat yoziladi).
type Settings struct {
	Provider Provider `json:"provider"`
	APIKey   string   `json:"api_key"`
	Model    string   `json:"model"`
	BaseURL  string   `json:"base_url,omitempty"`
	Password string   `json:"password,omitempty"` // bo'sh = o'zgartirmaydi
}

// stored — diskdagi ko'rinish (sirlar hex'da).
type stored struct {
	Provider   Provider `json:"provider"`
	APIKey     string   `json:"api_key"`
	Model      string   `json:"model"`
	BaseURL    string   `json:"base_url"`
	PassHash   string   `json:"pass_hash,omitempty"`
	Salt       string   `json:"salt,omitempty"`
	AuthSecret string   `json:"auth_secret,omitempty"`
}

// Status — UI uchun xavfsiz holat (kalit/parol maskalangan).
type Status struct {
	Enabled  bool     `json:"enabled"`
	Provider Provider `json:"provider"`
	Model    string   `json:"model"`
	BaseURL  string   `json:"base_url"`
	KeySet   bool     `json:"key_set"`
	KeyHint  string   `json:"key_hint,omitempty"`
	Auth     bool     `json:"auth"` // parol o'rnatilganmi
}

type Manager struct {
	mu       sync.RWMutex
	provider Provider
	apiKey   string
	model    string
	baseURL  string

	passHash   []byte
	salt       []byte
	authSecret []byte

	store  string
	client *http.Client
}

func New() *Manager {
	store := strings.TrimSpace(os.Getenv("AGENT_STORE"))
	if store == "" {
		store = "agent.json"
	}
	// So'rov timeouti — sekin/reasoning modellar uchun AGENT_TIMEOUT_SEC bilan
	// oshirish mumkin (standart 180s). Provayder umuman javob bermasa (tarmoq
	// yoki bloklangan bo'lsa) baribir shu vaqtdan keyin "deadline exceeded" beradi.
	timeout := 180 * time.Second
	if v := strings.TrimSpace(os.Getenv("AGENT_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	m := &Manager{
		provider: Provider(strings.TrimSpace(os.Getenv("AGENT_PROVIDER"))),
		apiKey:   strings.TrimSpace(os.Getenv("AGENT_API_KEY")),
		model:    strings.TrimSpace(os.Getenv("AGENT_MODEL")),
		baseURL:  strings.TrimSpace(os.Getenv("AGENT_BASE_URL")),
		store:    store,
		client:   &http.Client{Timeout: timeout},
	}
	if m.provider == "" {
		m.provider = ProviderAnthropic
	}
	if pw := strings.TrimSpace(os.Getenv("AGENT_PASSWORD")); pw != "" {
		m.setPassword(pw)
	}
	return m
}

func (m *Manager) LoadStore() {
	b, err := os.ReadFile(m.store)
	if err != nil {
		return
	}
	var s stored
	if err := json.Unmarshal(b, &s); err != nil {
		log.Printf("[agent] sozlama fayli buzuq (%s): %v", m.store, err)
		return
	}
	m.mu.Lock()
	if s.Provider != "" {
		m.provider = s.Provider
	}
	m.apiKey, m.model, m.baseURL = s.APIKey, s.Model, s.BaseURL
	m.passHash, _ = hex.DecodeString(s.PassHash)
	m.salt, _ = hex.DecodeString(s.Salt)
	m.authSecret, _ = hex.DecodeString(s.AuthSecret)
	m.mu.Unlock()
}

func (m *Manager) resolved() (Provider, string, string, string) {
	p := m.provider
	d := defaults[p]
	model := m.model
	if model == "" {
		model = d.model
	}
	base := m.baseURL
	if base == "" {
		base = d.baseURL
	}
	return p, m.apiKey, model, base
}

func (m *Manager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.provider == ProviderLocal {
		return true
	}
	return m.apiKey != ""
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// RAW qiymatlar — foydalanuvchi aynan nima kiritgan bo'lsa shu. Default
	// (masalan anthropic URL yoki model) UI'ga to'ldirilmaydi; u faqat haqiqiy
	// API chaqiruvida resolved() orqali qo'llanadi. Shunda base_url va model
	// maydonlari kiritilmaguncha bo'sh turadi.
	st := Status{
		Enabled: m.Enabled(), Provider: m.provider, Model: m.model,
		BaseURL: m.baseURL, KeySet: m.apiKey != "", Auth: len(m.passHash) > 0,
	}
	if n := len(m.apiKey); n >= 4 {
		st.KeyHint = "…" + m.apiKey[n-4:]
	}
	return st
}

// setPassword parolni tuzlab (salt) SHA-256 bilan saqlaydi va auth sirini yaratadi.
// Chaqiruvchi qulfni ushlamasligi kerak (o'zi Lock qiladi emas — New/SetConfig ichida).
func (m *Manager) setPassword(pw string) {
	salt := make([]byte, 16)
	rand.Read(salt)
	h := sha256.Sum256(append(salt, []byte(pw)...))
	m.salt = salt
	m.passHash = h[:]
	if len(m.authSecret) == 0 {
		m.authSecret = make([]byte, 32)
		rand.Read(m.authSecret)
	}
}

func (m *Manager) SetConfig(s Settings) error {
	m.mu.Lock()
	if s.Provider != "" {
		m.provider = s.Provider
	}
	if strings.TrimSpace(s.APIKey) != "" {
		m.apiKey = strings.TrimSpace(s.APIKey)
	}
	m.model = strings.TrimSpace(s.Model)
	m.baseURL = strings.TrimSpace(s.BaseURL)
	if strings.TrimSpace(s.Password) != "" {
		m.setPassword(strings.TrimSpace(s.Password))
	}
	snap := stored{
		Provider: m.provider, APIKey: m.apiKey, Model: m.model, BaseURL: m.baseURL,
		PassHash: hex.EncodeToString(m.passHash), Salt: hex.EncodeToString(m.salt),
		AuthSecret: hex.EncodeToString(m.authSecret),
	}
	m.mu.Unlock()

	b, _ := json.MarshalIndent(snap, "", "  ")
	if err := os.WriteFile(m.store, b, 0o600); err != nil {
		return fmt.Errorf("saqlab bo'lmadi: %w", err)
	}
	return nil
}

// --- Auth (parol -> token) ---

// AuthEnabled — parol o'rnatilganmi (o'rnatilmagan bo'lsa oqim ochiq).
func (m *Manager) AuthEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.passHash) > 0
}

// token — auth sirining HMAC'i (parolni bilgan qurilma uchun bearer).
func (m *Manager) token() string {
	if len(m.authSecret) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, m.authSecret)
	mac.Write([]byte("pulse-agent-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// Login parolni tekshiradi; to'g'ri bo'lsa qurilma saqlaydigan tokenni qaytaradi.
func (m *Manager) Login(pw string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.passHash) == 0 {
		return "", false
	}
	h := sha256.Sum256(append(append([]byte{}, m.salt...), []byte(pw)...))
	if subtle.ConstantTimeCompare(h[:], m.passHash) != 1 {
		return "", false
	}
	return m.token(), true
}

// ValidToken — token to'g'rimi (auth yoqilmagan bo'lsa har doim true).
func (m *Manager) ValidToken(t string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.passHash) == 0 {
		return true
	}
	want := m.token()
	return want != "" && subtle.ConstantTimeCompare([]byte(t), []byte(want)) == 1
}

// --- Kalitni tekshirish (avvalgidek) ---

func (m *Manager) Test(ctx context.Context) error {
	m.mu.RLock()
	p, key, model, base := m.resolved()
	m.mu.RUnlock()
	if p != ProviderLocal && key == "" {
		return errors.New("API kalit kiritilmagan")
	}
	if p == ProviderAnthropic {
		return m.testAnthropic(ctx, key, model, base)
	}
	return m.testOpenAICompatible(ctx, key, base)
}

func (m *Manager) testAnthropic(ctx context.Context, key, model, base string) error {
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 1,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", strings.NewReader(string(body)))
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	return m.doTest(req)
}

func (m *Manager) testOpenAICompatible(ctx context.Context, key, base string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return m.doTest(req)
}

func (m *Manager) doTest(req *http.Request) error {
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("kalit rad etildi (401/403)")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
