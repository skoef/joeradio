package provider

import (
	"context"
	"errors"
)

var (
	ErrSongNotFound   = errors.New("song not found")
	ErrRefreshedToken = errors.New("token was refreshed")
)

// Provider describes the functions for any provider
type Provider interface {
	// Name returns the provider's name
	Name() string
	// Authenticate handles authentication process of the provider
	Authenticate(ctx context.Context) error
	// GetFullPlaylist returns the playlist
	GetFullPlaylist(ctx context.Context) (*Playlist, error)
	// Search performs query and returns the top result
	Search(ctx context.Context, query string) (Track, error)
	// AddToPlaylist adds given track to playlist
	AddToPlaylist(ctx context.Context, trackID string) error
}

// Track represents a generic track
type Track interface {
	// GetTitle returns the track's title
	GetTitle() string
	// GetArtists returns a list of artist names
	GetArtists() []string
	// GetID returns the provider's unique ID for that the track
	GetID() string
}
