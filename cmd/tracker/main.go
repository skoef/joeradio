package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const (
	streamUrl   = "https://icecast-qmusicnl-cdp.triple-it.nl/Joe_nl_high.aac"
	playlistID  = "4t9w0OuAKt9mMEY27m1IDJ"
	redirectURI = "http://localhost:8080/callback"
)

var (
	auth = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithClientID(os.Getenv("SPOTIFY_CLIENT_ID")),
		spotifyauth.WithClientSecret(os.Getenv("SPOTIFY_CLIENT_SECRET")),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic))

	state string
	ch    = make(chan *spotify.Client)

	ignore = []string{
		"JOE nieuws",
		"Ad break",
	}

	authToken *oauth2.Token
)

func main() {
	ctx := context.Background()

	// seed RNG and create a state for authentication
	rand.New(rand.NewSource(time.Now().UnixNano()))
	state = strconv.Itoa(rand.Int())

	// first start an HTTP server
	http.HandleFunc("/callback", completeAuth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})

	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client := <-ch

	playlist, err := client.GetPlaylist(ctx, playlistID)
	if err != nil {
		panic("error: " + err.Error())
	}

	playlistMap := make(map[string]bool)
	for _, item := range playlist.Tracks.Tracks {
		playlistMap[string(item.Track.ID)] = true
	}

	fmt.Println("loaded playlist items", len(playlistMap))

	for {
		title, err := GetStreamTitle(streamUrl)
		if err != nil {
			fmt.Printf("error: %s\n", err)
		} else if title == "" {
			fmt.Printf("error: empty title\n")
		} else if shouldIgnoreTitle(title) {
			fmt.Printf("error: ignoring title %s\n", title)
		} else {
			fmt.Println("icecast title", title)

			results, err := client.Search(ctx, title, spotify.SearchTypeTrack, spotify.Limit(1))
			if err != nil {
				fmt.Printf("error: %s\n", err)
			} else if len(results.Tracks.Tracks) != 1 {
				fmt.Printf("no results for %s\n", title)
			} else {
				track := results.Tracks.Tracks[0]
				fmt.Printf("found %s by %s (ID: %s)\n", track.Name, artistNames(track.Artists), track.ID)

				if _, ok := playlistMap[string(track.ID)]; ok {
					fmt.Printf("track %s already on playlist, skipping\n", track.ID)
				} else {
					fmt.Printf("adding track %s to playlist\n", track.ID)

					_, err = client.AddTracksToPlaylist(ctx, playlistID, track.ID)
					if err != nil {
						if strings.Contains(err.Error(), `Post "https://accounts.spotify.com/api/token": context canceled`) {
							fmt.Println("token expired, trying to refresh")

							if tok, err := auth.RefreshToken(ctx, authToken); err != nil {
								fmt.Printf("error: refreshing token failed: %s\n", err)
							} else {
								fmt.Printf("refreshed auth token, retrying")

								authToken = tok

								continue
							}
						}

						fmt.Printf("error: %s\n", err)
					} else {
						playlistMap[string(track.ID)] = true
					}
				}
			}
		}

		time.Sleep(time.Minute)
	}
}

func artistNames(artists []spotify.SimpleArtist) string {
	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}

	return strings.Join(names, ",")
}

func shouldIgnoreTitle(title string) bool {
	for _, ig := range ignore {
		if title == ig {
			return true
		}
	}

	return false
}

func GetStreamTitle(streamUrl string) (string, error) {
	m, err := getStreamMetas(streamUrl)
	if err != nil {
		return "", err
	}
	// Should be at least "StreamTitle=' '"
	if len(m) < 15 {
		return "", nil
	}
	// Split meta by ';', trim it and search for StreamTitle
	for _, m := range bytes.Split(m, []byte(";")) {
		m = bytes.Trim(m, " \t")
		if !bytes.Equal(m[0:13], []byte("StreamTitle='")) {
			continue
		}

		return string(m[13 : len(m)-1]), nil
	}

	return "", errors.New("no stream title")
}

func getStreamMetas(streamUrl string) ([]byte, error) {
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, streamUrl, nil)
	req.Header.Set("Icy-Metadata", "1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// We sent "Icy-MetaData", we should have a "icy-metaint" in return
	ih := resp.Header.Get("Icy-Metaint")
	if ih == "" {
		return nil, errors.New("no metadata")
	}
	// "icy-metaint" is how often (in bytes) should we receive the meta
	ib, err := strconv.Atoi(ih)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(resp.Body)

	// skip the first mp3 frame
	c, err := reader.Discard(ib)
	if err != nil {
		return nil, err
	}
	// If we didn't received ib bytes, the stream is ended
	if c != ib {
		return nil, errors.New("stream ended prematurally")
	}

	// get the size byte, that is the metadata length in bytes / 16
	sb, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	ms := int(sb * 16)

	// read the ms first bytes it will contain metadata
	m, err := reader.Peek(ms)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	var err error

	authToken, err = auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}

	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}

	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), authToken))
	_, _ = fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
