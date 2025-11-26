package spotify

import "github.com/zmb3/spotify/v2"

// Track implements provider.Track
type Track struct {
	track spotify.FullTrack
}

func newTrack(track *spotify.FullTrack) *Track {
	return &Track{
		track: *track,
	}
}

// GetID returns the track's unique ID
func (t *Track) GetID() string {
	return string(t.track.ID)
}

// GetTitle returns the track's title, or name as spotify calls it
func (t *Track) GetTitle() string {
	return t.track.Name
}

// GetArtists returns the track's artists
func (t *Track) GetArtists() []string {
	artists := make([]string, len(t.track.Artists))
	for i, a := range t.track.Artists {
		artists[i] = a.Name
	}

	return artists
}
