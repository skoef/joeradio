package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// JoinMessage is the message we send to JoeRadio to "join" the subscription
	// for plays (songs) and send us already 1 song back
	JoinMessage = `["{\"action\":\"join\",\"id\":0,\"sub\":{\"station\":\"joe_nl\",\"entity\":\"plays\",\"action\":\"play\"},\"backlog\":1}"]`
)

// ParseAMessage parses an "a" message coming from the JoeRadio websocket stream
func ParseAMessage(input []byte) (*Song, error) {
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

	return &Song{
		Artist: data.Data.Artist.Name,
		Title:  data.Data.Title,
	}, nil
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

// Song holds song information relevant to searching on spotify
type Song struct {
	Artist string
	Title  string
}

func (s Song) String() string {
	return s.Artist + " - " + s.Title
}
