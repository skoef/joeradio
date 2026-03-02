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
	SpotifyAuthHost     string
	SpotifyAuthPort     int
}

// NewDefaultConfig returns default config
func NewDefaultConfig() Config {
	cwd, _ := os.Getwd()

	return Config{
		Provider:         "spotify", // currently only supported
		SpotifyTokenPath: cwd,
	}
}

// Validate returns an error when the configuration doesn't validate
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

		if c.SpotifyAuthPort < 0 || c.SpotifyAuthPort > 65535 {
			return errors.New("-spotify-auth-port should be between 0 and 65535")
		}

	default:
		return fmt.Errorf("unknown provider: %s", c.Provider)
	}

	return nil
}
