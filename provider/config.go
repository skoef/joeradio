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
}

// NewDefaultConfig returns default config
func NewDefaultConfig() Config {
	cwd, _ := os.Getwd()

	return Config{
		Provider:         "spotify", // currently only supported
		SpotifyTokenPath: cwd,
	}
}

func (c Config) Validate() error {
	// check for provider settings
	switch c.Provider {
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

	default:
		return fmt.Errorf("unknown provider: %s", c.Provider)
	}

	return nil
}
