package main

import (
	"errors"
	"testing"

	"joeradio/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zmb3/spotify/v2"
)

func TestGetFullPlaylist(t *testing.T) {
	testPlaylist := "foo"
	testPlaylistID := spotify.ID(testPlaylist)

	t.Run("API error", func(t *testing.T) {
		client := mocks.NewMockSpotifyClient(t)

		someError := errors.New("API error")
		client.EXPECT().GetPlaylistItems(t.Context(), testPlaylistID, mock.Anything).
			Return(nil, someError)

		pl, err := getFullPlaylist(t.Context(), client, testPlaylist)
		assert.Nil(t, pl)
		if assert.Error(t, err) {
			assert.ErrorIs(t, err, someError)
		}
	})

	t.Run("success", func(t *testing.T) {
		client := mocks.NewMockSpotifyClient(t)

		// first call
		firstReponse := &spotify.PlaylistItemPage{}
		firstTrack := &spotify.FullTrack{}
		firstTrack.ID = spotify.ID("track1")
		firstReponse.Items = []spotify.PlaylistItem{{Track: spotify.PlaylistItemTrack{Track: firstTrack}}}
		firstReponse.Total = 2
		client.EXPECT().GetPlaylistItems(t.Context(), testPlaylistID, mock.Anything).
			Return(firstReponse, nil).
			Once()

		// second call
		secondReponse := &spotify.PlaylistItemPage{}
		secondTrack := &spotify.FullTrack{}
		secondTrack.ID = spotify.ID("track2")
		secondReponse.Items = []spotify.PlaylistItem{{Track: spotify.PlaylistItemTrack{Track: secondTrack}}}
		secondReponse.Total = 2
		client.EXPECT().GetPlaylistItems(t.Context(), testPlaylistID, mock.Anything).
			Return(secondReponse, nil).
			Once()

		pl, err := getFullPlaylist(t.Context(), client, testPlaylist)
		require.NoError(t, err)
		assert.Equal(t, pl.Len(), 2)
	})
}
