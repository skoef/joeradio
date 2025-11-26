package source

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAMessage(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/message.txt")
	require.NoError(t, err)

	song, err := ParseAMessage(data)
	require.NoError(t, err)

	assert.Equal(t, "WHITESNAKE", song.GetArtists()[0])
	assert.Equal(t, "Here I Go Again", song.GetTitle())
}
