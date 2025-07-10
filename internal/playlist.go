package internal

import (
	"log/slog"
	"sync"
)

type Playlist struct {
	list map[string]bool

	lock sync.RWMutex
}

func NewPlaylist(items []string) *Playlist {
	p := &Playlist{}
	for _, i := range items {
		p.Add(i)
	}

	return p
}

func (p *Playlist) Add(trackID string) int {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.list == nil {
		p.list = make(map[string]bool)
	}

	_, found := p.list[trackID]
	if found {
		slog.Warn("duplicate item in playlist", slog.String("track", trackID))
	}

	p.list[trackID] = true
	return len(p.list)
}

func (p *Playlist) Has(trackID string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, found := p.list[trackID]
	return found
}

func (p *Playlist) Len() int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return len(p.list)
}
