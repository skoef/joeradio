// Package source implements the websocket source for the tracks stream
package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// WebsocketURL is the URL for Joe Radio NL's websocket
	// This URL is also used in the "listen now" popup on their website
	WebsocketURL = "wss://socket.qmusic.be/api/502/ltfn4msd/websocket"

	// JoinMessage is the message we send to JoeRadio to "join" the subscription
	// for plays (songs) and send us already 1 song back
	JoinMessage = `["{\"action\":\"join\",\"id\":0,\"sub\":{\"station\":\"joe_nl\",\"entity\":\"plays\",\"action\":\"play\"},\"backlog\":1}"]`
)

// ParseAMessage parses an "a" message coming from the JoeRadio websocket stream
func ParseAMessage(input []byte) (*SimpleTrack, error) {
	msg := strings.TrimSpace(string(input))
	if !strings.HasPrefix(msg, "a[") || !strings.HasSuffix(msg, "]") {
		return nil, errors.New("not an A message")
	}

	msg = strings.TrimPrefix(msg, "a[")
	msg = strings.TrimSuffix(msg, "]")

	msg, err := strconv.Unquote(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to unquote message: %w", err)
	}

	var resp websocketResponse
	if err = json.Unmarshal([]byte(msg), &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var data websocketData
	if err = json.Unmarshal([]byte(resp.Data), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return &SimpleTrack{
		artist: data.Data.Artist.Name,
		title:  data.Data.Title,
	}, nil
}

// SimpleTrack implements the provider.Track interface
type SimpleTrack struct {
	artist string
	title  string
}

// GetID should return the track's ID but it has none
func (SimpleTrack) GetID() string {
	// not implemented
	return ""
}

// GetTitle returns the track's title
func (d SimpleTrack) GetTitle() string {
	return d.title
}

// GetArtists returns the track's artists
func (d SimpleTrack) GetArtists() []string {
	return []string{d.artist}
}

type websocketResponse struct {
	Data string
}

//nolint:revive // this is just the structure of the JSON
type websocketData struct {
	Data struct {
		Title  string
		Artist struct {
			Name string
		}
	}
}
