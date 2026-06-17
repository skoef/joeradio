package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	mocks "joeradio/mocks/provider/spotify"
	"joeradio/provider"
)

const testPlaylistID = "test-playlist-id"

func nullLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestSpotify(client Client) *Spotify {
	return &Spotify{
		logger:        nullLogger(),
		playlistID:    spotify.ID(testPlaylistID),
		authenticator: spotifyauth.New(),
		client:        client,
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	cfg := provider.Config{
		Logger:              nullLogger(),
		SpotifyClientID:     "client-id",
		SpotifyClientSecret: "client-secret",
		SpotifyPlaylistID:   "playlist-id",
		SpotifyTokenPath:    "/tmp",
	}

	s, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, spotify.ID("playlist-id"), s.playlistID)
	assert.Equal(t, "/tmp", s.tokenPath)
	assert.NotNil(t, s.authenticator)
}

func TestName(t *testing.T) {
	t.Parallel()

	s := &Spotify{}
	assert.Equal(t, "spotify", s.Name())
}

func TestGetFullPlaylist(t *testing.T) {
	t.Parallel()

	t.Run("API error", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		someError := errors.New("API error")
		client.EXPECT().GetPlaylistItems(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return(nil, someError)

		pl, err := s.GetFullPlaylist(t.Context())
		assert.Nil(t, pl)
		require.Error(t, err)
		assert.ErrorIs(t, err, someError)
	})

	t.Run("single page", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		track := &spotify.FullTrack{}
		track.ID = spotify.ID("track1")
		response := &spotify.PlaylistItemPage{
			Items: []spotify.PlaylistItem{{Track: spotify.PlaylistItemTrack{Track: track}}},
		}
		response.Total = 1

		client.EXPECT().GetPlaylistItems(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return(response, nil)

		pl, err := s.GetFullPlaylist(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 1, pl.Len())
		assert.True(t, pl.Has("track1"))
	})

	t.Run("paginated", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		firstTrack := &spotify.FullTrack{}
		firstTrack.ID = spotify.ID("track1")
		firstResponse := &spotify.PlaylistItemPage{
			Items: []spotify.PlaylistItem{{Track: spotify.PlaylistItemTrack{Track: firstTrack}}},
		}
		firstResponse.Total = 2

		secondTrack := &spotify.FullTrack{}
		secondTrack.ID = spotify.ID("track2")
		secondResponse := &spotify.PlaylistItemPage{
			Items: []spotify.PlaylistItem{{Track: spotify.PlaylistItemTrack{Track: secondTrack}}},
		}
		secondResponse.Total = 2

		client.EXPECT().GetPlaylistItems(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return(firstResponse, nil).Once()
		client.EXPECT().GetPlaylistItems(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return(secondResponse, nil).Once()

		pl, err := s.GetFullPlaylist(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 2, pl.Len())
		assert.True(t, pl.Has("track1"))
		assert.True(t, pl.Has("track2"))
	})
}

func TestSearch(t *testing.T) {
	t.Parallel()

	t.Run("API error", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		someError := errors.New("search failed")
		client.EXPECT().Search(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, someError)

		track, err := s.Search(t.Context(), "some query")
		assert.Nil(t, track)
		require.Error(t, err)
		assert.ErrorIs(t, err, someError)
	})

	t.Run("context canceled triggers token refresh", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)
		s.token = &oauth2.Token{}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		client.EXPECT().Search(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, context.Canceled)

		_, err := s.Search(ctx, "some query")
		// refreshToken will fail since authenticator has no credentials,
		// but we verify the canceled path was taken (not the generic error path)
		require.Error(t, err)
		assert.NotErrorIs(t, err, context.Canceled)
	})

	t.Run("no results", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		result := &spotify.SearchResult{
			Tracks: &spotify.FullTrackPage{},
		}
		client.EXPECT().Search(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(result, nil)

		track, err := s.Search(t.Context(), "nonexistent song")
		assert.Nil(t, track)
		require.Error(t, err)
		assert.ErrorIs(t, err, provider.ErrSongNotFound)
	})

	t.Run("success appends year range to query", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		fullTrack := spotify.FullTrack{}
		fullTrack.ID = "track-id-123"
		fullTrack.Name = "Song Name"
		fullTrack.Artists = []spotify.SimpleArtist{{Name: "Artist One"}}

		result := &spotify.SearchResult{
			Tracks: &spotify.FullTrackPage{},
		}
		result.Tracks.Tracks = []spotify.FullTrack{fullTrack}

		client.EXPECT().Search(mock.Anything, "song title year:1970-1999", mock.Anything, mock.Anything).
			Return(result, nil)

		track, err := s.Search(t.Context(), "song title")
		require.NoError(t, err)
		require.NotNil(t, track)
		assert.Equal(t, "track-id-123", track.GetID())
		assert.Equal(t, "Song Name", track.GetTitle())
		assert.Equal(t, []string{"Artist One"}, track.GetArtists())
	})
}

func TestAddToPlaylist(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		client.EXPECT().AddTracksToPlaylist(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return("snapshot-id", nil)

		require.NoError(t, s.AddToPlaylist(t.Context(), "track-123"))
	})

	t.Run("API error", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)

		someError := errors.New("add tracks failed")
		client.EXPECT().AddTracksToPlaylist(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return("", someError)

		err := s.AddToPlaylist(t.Context(), "track-123")
		require.Error(t, err)
		assert.ErrorIs(t, err, someError)
	})

	t.Run("context canceled triggers token refresh", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockSpotifyClient(t)
		s := newTestSpotify(client)
		s.token = &oauth2.Token{}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		client.EXPECT().AddTracksToPlaylist(mock.Anything, spotify.ID(testPlaylistID), mock.Anything).
			Return("", context.Canceled)

		err := s.AddToPlaylist(ctx, "track-123")
		// refreshToken will fail since authenticator has no credentials,
		// but we verify the canceled path was taken (not nil return)
		require.Error(t, err)
		assert.NotErrorIs(t, err, context.Canceled)
	})
}

func TestAuthenticate(t *testing.T) {
	t.Parallel()

	t.Run("valid cached token", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		token := oauth2.Token{
			AccessToken: "test-access-token",
			Expiry:      time.Now().Add(time.Hour),
		}
		data, err := json.Marshal(token)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, tokenFilename), data, 0o600))

		s := &Spotify{
			logger:        nullLogger(),
			tokenPath:     dir,
			playlistID:    spotify.ID(testPlaylistID),
			authenticator: spotifyauth.New(),
		}

		require.NoError(t, s.Authenticate(t.Context()))
		assert.NotNil(t, s.client)
		assert.Equal(t, "test-access-token", s.token.AccessToken)
	})

	t.Run("invalid token JSON", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, tokenFilename), []byte("not valid json"), 0o600))

		s := &Spotify{
			logger:        nullLogger(),
			tokenPath:     dir,
			playlistID:    spotify.ID(testPlaylistID),
			authenticator: spotifyauth.New(),
		}

		err := s.Authenticate(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not unmarshal token")
	})
}
