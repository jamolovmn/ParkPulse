// Package logbuf har konteyner uchun oxirgi N ta xom log qatorini saqlaydi.
// Arvoh ochilish aniqlanganda "o'sha paytda logda nima bo'lgan edi" degan
// savolga javob — kontekst shu yerdan olinadi.
package logbuf

import "sync"

type Buffer struct {
	mu    sync.Mutex
	limit int
	lines map[string][]string // kalit: konteyner nomi
}

func New(limit int) *Buffer {
	return &Buffer{limit: limit, lines: make(map[string][]string)}
}

func (b *Buffer) Add(container, line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := append(b.lines[container], line)
	if len(s) > b.limit {
		s = s[len(s)-b.limit:]
	}
	b.lines[container] = s
}

// Snapshot hozirgi holatning nusxasini qaytaradi.
func (b *Buffer) Snapshot(container string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	src := b.lines[container]
	out := make([]string, len(src))
	copy(out, src)
	return out
}
