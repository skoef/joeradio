package spotify

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zmb3/spotify/v2"
)

func TestGetID(t *testing.T) {
	t.Parallel()

	ft := spotify.FullTrack{}
	ft.ID = "abc123"
	assert.Equal(t, "abc123", newTrack(ft).GetID())
}

func TestGetTitle(t *testing.T) {
	t.Parallel()

	ft := spotify.FullTrack{}
	ft.Name = "Blue Monday"
	assert.Equal(t, "Blue Monday", newTrack(ft).GetTitle())
}

func TestGetArtists(t *testing.T) {
	t.Parallel()

	t.Run("no artists", func(t *testing.T) {
		t.Parallel()

		ft := spotify.FullTrack{}
		assert.Empty(t, newTrack(ft).GetArtists())
	})

	t.Run("single artist", func(t *testing.T) {
		t.Parallel()

		ft := spotify.FullTrack{}
		ft.Artists = []spotify.SimpleArtist{{Name: "New Order"}}
		assert.Equal(t, []string{"New Order"}, newTrack(ft).GetArtists())
	})

	t.Run("multiple artists", func(t *testing.T) {
		t.Parallel()

		ft := spotify.FullTrack{}
		ft.Artists = []spotify.SimpleArtist{{Name: "Artist A"}, {Name: "Artist B"}}
		assert.Equal(t, []string{"Artist A", "Artist B"}, newTrack(ft).GetArtists())
	})
}
