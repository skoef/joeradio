package internal

import (
	"context"

	"github.com/zmb3/spotify/v2"
)

// SpotifyClient describes the functions we use on the spotify.Client so we can
// mock them for testing
type SpotifyClient interface {
	Search(ctx context.Context, query string, t spotify.SearchType, opts ...spotify.RequestOption) (*spotify.SearchResult, error)
	AddTracksToPlaylist(ctx context.Context, playlistID spotify.ID, trackIDs ...spotify.ID) (snapshotID string, err error)
	GetPlaylistItems(ctx context.Context, playlistID spotify.ID, opts ...spotify.RequestOption) (*spotify.PlaylistItemPage, error)
}

// GetFullPlaylist iterates over the playlist item pages returned by the API until
// all items are fetched
func GetFullPlaylist(ctx context.Context, client SpotifyClient, playlistID string) (*Playlist, error) {
	playlist := NewPlaylist()
	offset := 0

	for {
		playlistItems, err := client.GetPlaylistItems(ctx, spotify.ID(playlistID), spotify.Offset(offset))
		if err != nil {
			return nil, err
		}

		offset += len(playlistItems.Items)

		for _, item := range playlistItems.Items {
			playlist.Add(string(item.Track.Track.ID))
		}

		// are we done yet?
		if offset >= int(playlistItems.Total) {
			break
		}
	}

	return playlist, nil
}
