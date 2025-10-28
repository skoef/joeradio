package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zmb3/spotify/v2"
)

func TestArtistNames(t *testing.T) {
	t.Parallel()

	names := []spotify.SimpleArtist{
		{Name: "Whitney Houston"},
		{Name: "Pia Zadora"},
	}

	assert.Equal(t, "Whitney Houston,Pia Zadora", artistNames(names))
}
