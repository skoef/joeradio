package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlayList(t *testing.T) {
	pl := NewPlaylist("foo", "bar", "bar")

	assert.Len(t, pl.list, 2)

	pl = NewPlaylist()
	assert.Equal(t, 1, pl.Add("foo"))
	assert.Equal(t, 2, pl.Add("bar"))
	assert.Equal(t, 2, pl.Add("bar"))

	assert.Equal(t, 2, pl.Len())

	assert.True(t, pl.Has("foo"))
	assert.True(t, pl.Has("bar"))
	assert.False(t, pl.Has("baz"))
}
