// Package collector Docker API orqali konteyner loglarini jonli (follow) o'qiydi.
// Postgres'ga umuman tegmaydi — faqat log stream.
package collector

import (
	"bufio"
	"context"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"io"
	"log"
	"time"

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

type Collector struct {
	cli       *client.Client
	names     []string
	Events    chan *parser.Event
	Buf       *logbuf.Buffer
	HealthOut chan Health
}

func New(names []string) (*Collector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Collector{
		cli:       cli,
		names:     names,
		Events:    make(chan *parser.Event, 256),
		HealthOut: make(chan Health, 16),
	}, nil
}

// Run har bir konteyner uchun alohida goroutine'da tail boshlaydi.
func (c *Collector) Run(ctx context.Context) {
	for _, name := range c.names {
		go c.tailLoop(ctx, name)
	}
	go c.systemHealthLoop(ctx)
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
		if c.Buf != nil {
			c.Buf.Add(name, parser.Clean(line))
		}
		if ev := parser.Parse(name, line); ev != nil {
			c.Events <- ev
		}
	}
	return scanner.Err()
}
