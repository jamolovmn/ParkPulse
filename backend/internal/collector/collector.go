// Package collector Docker API orqali konteyner loglarini jonli (follow) o'qiydi.
// Postgres'ga umuman tegmaydi — faqat log stream.
package collector

import (
	"bufio"
	"context"
	"io"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"parkpulse/backend/internal/logbuf"
	"parkpulse/backend/internal/parser"
)

type Collector struct {
	cli    *client.Client
	names  []string // kuzatiladigan konteyner nomlari
	Events chan *parser.Event
	Buf    *logbuf.Buffer // barcha xom qatorlar (arvoh konteksti uchun)
}

func New(names []string) (*Collector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Collector{cli: cli, names: names, Events: make(chan *parser.Event, 256)}, nil
}

// Run har bir konteyner uchun alohida goroutine'da tail boshlaydi.
func (c *Collector) Run(ctx context.Context) {
	for _, name := range c.names {
		go c.tailLoop(ctx, name)
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
		if c.Buf != nil {
			c.Buf.Add(name, parser.Clean(line))
		}
		if ev := parser.Parse(name, line); ev != nil {
			c.Events <- ev
		}
	}
	return scanner.Err()
}
