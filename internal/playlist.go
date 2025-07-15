// Package internal holds tooling
package internal

import (
	"log/slog"
	"sync"
)

// Playlist is a concurrent safe wrapper for a unique list of tracks
type Playlist struct {
	list map[string]bool

	lock sync.RWMutex
}

// NewPlaylist returns a new Playlist and fills it with given items
func NewPlaylist(items []string) *Playlist {
	p := &Playlist{}
	for _, i := range items {
		p.Add(i)
	}

	return p
}

// Add adds unique trackID in to Playlist and returns new number of items in the
// Playlist
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

// Has returns true if trackID is found in Playlist
func (p *Playlist) Has(trackID string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, found := p.list[trackID]

	return found
}

// Len returns number of items in Playlist
func (p *Playlist) Len() int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return len(p.list)
}
