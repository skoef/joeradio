package applemusic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	api "github.com/minchao/go-apple-music"

	"joeradio/provider"
)

// AppleMusic implements the Provider interface for Apple Music
type AppleMusic struct {
	logger     *slog.Logger
	token      string
	playlistID string
	client     *api.Client
}

// New returns new Apple Music provider
func New(config provider.Config) (*AppleMusic, error) {
	return &AppleMusic{
		logger:     config.Logger.With(slog.String("provider", "applemusic")),
		token:      config.AppleMusicAPIToken,
		playlistID: config.AppleMusicPlaylistID,
	}, nil
}

// Name returns the provider's name
func (*AppleMusic) Name() string {
	return "applemusic"
}

// Authenticate handles authentication process of the provider
func (a *AppleMusic) Authenticate(context.Context) error {
	tp := api.Transport{Token: a.token}

	a.client = api.NewClient(tp.Client())

	return nil
}

// GetFullPlaylist returns the playlist
func (a *AppleMusic) GetFullPlaylist(ctx context.Context) (*provider.Playlist, error) {
	a.logger.Debug("fetching playlist", slog.String("playlist", a.playlistID))

	// TODO: check if we need to iterate over multiple pages of results
	songs, _, err := a.client.Me.GetLibraryPlaylistTracks(ctx, a.playlistID, &api.PageOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not get library playlist tracks: %w", err)
	}

	playlist := provider.NewPlaylist()
	for i := range songs {
		playlist.Add(songs[i].Id)
	}

	return playlist, nil
}

// Search performs query and returns the top result
func (a *AppleMusic) Search(ctx context.Context, query string) (provider.Track, error) {
	return nil, errors.New("not implemented")
}

// AddToPlaylist adds given track to playlist
func (a *AppleMusic) AddToPlaylist(ctx context.Context, trackID string) error {
	return errors.New("not implemented")
}
