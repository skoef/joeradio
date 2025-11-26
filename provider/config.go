package provider

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// Config holds provider-wide configuration
type Config struct {
	// Logging config
	Debug  bool
	Logger *slog.Logger

	// name of configured provider
	Provider string

	// Spotify specific config
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyTokenPath    string
	SpotifyPlaylistID   string

	// AppleMusic specific config
	AppleMusicAPIToken   string
	AppleMusicPlaylistID string
}

// NewDefaultConfig returns default config
func NewDefaultConfig() Config {
	cwd, _ := os.Getwd()

	return Config{
		SpotifyTokenPath: cwd,
	}
}

// Validate checks if the configuration is valid and returns an error otherwise
func (c Config) Validate() error {
	// check for provider settings
	switch c.Provider {
	case "applemusic":
		if c.AppleMusicAPIToken == "" {
			return errors.New("provide -apple-music-api-token")
		}

		if c.AppleMusicPlaylistID == "" {
			return errors.New("provide -apple-music-playlist-id")
		}

	case "spotify":
		if c.SpotifyClientID == "" {
			return errors.New("provide -spotify-client-id")
		}

		if c.SpotifyClientSecret == "" {
			return errors.New("provide -spotify-client-secret")
		}

		if c.SpotifyPlaylistID == "" {
			return errors.New("provide -spotify-playlist-id")
		}

	case "":
		return errors.New("no provider chosen")
	default:
		return fmt.Errorf("unknown provider: %s", c.Provider)
	}

	return nil
}
