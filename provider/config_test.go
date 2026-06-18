package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := NewDefaultConfig()
	assert.Equal(t, "spotify", cfg.Provider)
	assert.NotEmpty(t, cfg.SpotifyTokenPath)
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("valid spotify config", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			Provider:            "spotify",
			SpotifyClientID:     "client-id",
			SpotifyClientSecret: "client-secret",
			SpotifyPlaylistID:   "playlist-id",
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("missing client ID", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			Provider:            "spotify",
			SpotifyClientSecret: "client-secret",
			SpotifyPlaylistID:   "playlist-id",
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spotify-client-id")
	})

	t.Run("missing client secret", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			Provider:          "spotify",
			SpotifyClientID:   "client-id",
			SpotifyPlaylistID: "playlist-id",
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spotify-client-secret")
	})

	t.Run("missing playlist ID", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			Provider:            "spotify",
			SpotifyClientID:     "client-id",
			SpotifyClientSecret: "client-secret",
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spotify-playlist-id")
	})

	t.Run("unknown provider", func(t *testing.T) {
		t.Parallel()

		cfg := Config{Provider: "unknown"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider")
	})
}
