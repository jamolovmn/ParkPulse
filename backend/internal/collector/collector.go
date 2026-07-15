// Package collector Docker API orqali konteyner loglarini jonli (follow) o'qiydi.
// Postgres'ga umuman tegmaydi — faqat log stream.
package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"parkpulse/backend/internal/detector"
	"parkpulse/backend/internal/logbuf"
	"parkpulse/backend/internal/parser"
)

type ContainerStat struct {
	Name   string  `json:"name"`
	CPU    float64 `json:"cpu_percent"`
	RAM    float64 `json:"ram_percent"`
	RAM_MB float64 `json:"ram_mb"`
}

type Health struct {
	UptimeSec  float64         `json:"uptime_sec"`
	Cores      []float64       `json:"cores"`
	Containers []ContainerStat `json:"containers"`
	TotalRAM   float64         `json:"total_ram_mb"`
	UsedRAM    float64         `json:"used_ram_mb"`
}

// ContainerInfo — UI'da tanlash uchun konteyner haqida qisqa ma'lumot.
type ContainerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Watched bool   `json:"watched"` // hozir loglari o'qilyaptimi
}

type Collector struct {
	cli       *client.Client
	Events    chan *parser.Event
	Buf       *logbuf.Buffer
	Detector  *detector.Detector // aqlli o'qish (ixtiyoriy); nil bo'lsa oddiy parser
	HealthOut chan Health

	initial []string // env/config'dan kelgan boshlang'ich nomlar
	store   string   // UI orqali tanlangan konteynerlar saqlanadigan fayl

	mu    sync.Mutex
	root  context.Context               // Run'dan olingan asosiy ctx (yangi tail'lar uchun)
	tails map[string]context.CancelFunc // kalit: konteyner nomi -> to'xtatish
}

func New(names []string) (*Collector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	store := strings.TrimSpace(os.Getenv("TARGET_STORE"))
	if store == "" {
		store = "target.json"
	}
	return &Collector{
		cli:       cli,
		initial:   names,
		store:     store,
		Events:    make(chan *parser.Event, 256),
		HealthOut: make(chan Health, 16),
		tails:     make(map[string]context.CancelFunc),
	}, nil
}

// Run tail'larni boshlaydi. UI orqali saqlangan tanlov bo'lsa o'shani, bo'lmasa
// env/config'dagi nomlarni ishlatadi.
func (c *Collector) Run(ctx context.Context) {
	c.mu.Lock()
	c.root = ctx
	c.mu.Unlock()

	targets := c.loadTargets()
	if len(targets) == 0 {
		targets = c.initial
	}
	c.SetTargets(targets)
	go c.systemHealthLoop(ctx)
}

// Targets hozir kuzatilayotgan konteyner nomlarini qaytaradi.
func (c *Collector) Targets() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.tails))
	for name := range c.tails {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SetTargets kuzatiladigan konteynerlar to'plamini yangilaydi: olib tashlanganlar
// to'xtatiladi, yangilari boshlanadi. Tanlov diskka saqlanadi (restartda tiklanadi).
func (c *Collector) SetTargets(names []string) {
	want := make(map[string]bool)
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}

	c.mu.Lock()
	// Olib tashlanganlarni to'xtatamiz.
	for name, cancel := range c.tails {
		if !want[name] {
			cancel()
			delete(c.tails, name)
		}
	}
	// Yangilarini boshlaymiz (root ctx hali bo'lmasa — Run keyinroq boshlaydi).
	if c.root != nil {
		for name := range want {
			if _, ok := c.tails[name]; !ok {
				cctx, cancel := context.WithCancel(c.root)
				c.tails[name] = cancel
				go c.tailLoop(cctx, name)
			}
		}
	}
	c.saveTargets(want)
	c.mu.Unlock()
}

// ListContainers Docker'dagi ishlab turgan konteynerlarni qaytaradi (UI tanlovi).
func (c *Collector) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	list, err := c.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}
	watched := make(map[string]bool)
	for _, n := range c.Targets() {
		watched[n] = true
	}
	out := make([]ContainerInfo, 0, len(list))
	for _, ct := range list {
		name := ""
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		id := ct.ID
		if len(id) > 12 {
			id = id[:12]
		}
		out = append(out, ContainerInfo{
			ID: id, Name: name, Image: ct.Image, State: ct.State, Watched: watched[name],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// loadTargets/saveTargets — UI tanlovini diskda saqlaydi.
func (c *Collector) loadTargets() []string {
	b, err := os.ReadFile(c.store)
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		log.Printf("[collector] target fayli buzuq (%s): %v", c.store, err)
		return nil
	}
	return names
}

// saveTargets chaqiruvchi c.mu ni ushlab turishi shart.
func (c *Collector) saveTargets(set map[string]bool) {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	b, _ := json.MarshalIndent(names, "", "  ")
	if err := os.WriteFile(c.store, b, 0o600); err != nil {
		log.Printf("[collector] target tanlovini saqlab bo'lmadi (%s): %v", c.store, err)
	}
}

// tailLoop stream uzilsa (konteyner restart va h.k.) 3 soniyadan keyin qayta ulanadi.
func (c *Collector) tailLoop(ctx context.Context, name string) {
	for {
		if err := c.tail(ctx, name); err != nil && ctx.Err() == nil {
			log.Printf("[collector] %s: %v — 3s dan keyin qayta ulanish", name, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (c *Collector) tail(ctx context.Context, name string) error {
	inspect, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return err
	}

	rc, err := c.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true, // latency o'lchash uchun Docker'ning ns-aniqlikdagi vaqti
		Tail:       "0",  // faqat yangi loglar, tarix emas
	})
	if err != nil {
		return err
	}
	defer rc.Close()

	// TTY yoqilmagan konteynerlarda stream multiplexed keladi — stdcopy bilan ajratamiz.
	var reader io.Reader = rc
	if !inspect.Config.Tty {
		pr, pw := io.Pipe()
		go func() {
			_, err := stdcopy.StdCopy(pw, pw, rc)
			pw.CloseWithError(err)
		}()
		reader = pr
	}

	log.Printf("[collector] %s: tail boshlandi", name)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		cleaned := parser.Clean(line)
		if c.Buf != nil {
			c.Buf.Add(name, cleaned)
		}
		kind, ev := parser.Detect(name, line)
		if c.Detector != nil {
			for _, e := range c.Detector.Feed(name, cleaned, parser.TimeOf(line), kind, ev) {
				c.Events <- e
			}
		} else if ev != nil {
			c.Events <- ev
		}
	}
	return scanner.Err()
}
