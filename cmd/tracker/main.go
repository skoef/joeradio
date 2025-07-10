package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"joeradio/internal"
	"log/slog"
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
	errRefreshedToken = errors.New("token was refreshed")

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

	client        *spotify.Client
	authToken     *oauth2.Token
	playlistCache *internal.Playlist
	lastTitle     string
)

func main() {
	// set up logging
	logOpts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, logOpts))
	slog.SetDefault(logger)

	ctx := context.Background()

	// seed RNG and create a state for authentication
	rand.New(rand.NewSource(time.Now().UnixNano()))
	state = strconv.Itoa(rand.Int())

	// first start an HTTP server
	http.HandleFunc("/callback", completeAuth)

	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			slog.Error("failed serving oAuth callback", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	url := auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client = <-ch

	var err error
	playlistCache, err = getFullPlaylist(ctx, playlistID)
	if err != nil {
		slog.Error("failed to fetch playlist", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Debug("loaded playlist", slog.Int("items", playlistCache.Len()))
	logger.Debug("start tracking icecast", slog.String("url", streamUrl))

	for {
		if err := run(ctx); err != nil {
			logger.Warn("failed to complete run", slog.String("error", err.Error()))
			if errors.Is(err, errRefreshedToken) {
				logger.Debug("retrying immediately")
				continue
			}
		}

		time.Sleep(time.Minute)
	}
}

func run(ctx context.Context) error {
	title, err := GetStreamTitle(streamUrl)
	if err != nil {
		return err
	}

	if title == "" {
		return errors.New("empty title")
	}

	logger := slog.With(slog.String("title", title))
	if shouldIgnoreTitle(title) {
		logger.Info("ignoring")
		return nil
	}

	if title == lastTitle {
		logger.Debug("same song, waiting")
		return nil
	}

	logger.Info("search title on spotify")
	// limit search to the range of 1970 until 1999, since Joe is a station dedicated
	// to 70's, 80's and 90's music
	// this prevents us from getting remixes from later years in the results
	results, err := client.Search(ctx, fmt.Sprintf("%s year:1970-1999", title), spotify.SearchTypeTrack, spotify.Limit(1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Warn("token expired, trying to refresh")

			return refreshToken(ctx)
		}

		return fmt.Errorf("could not search: %w", err)
	}

	if len(results.Tracks.Tracks) == 0 {
		lastTitle = title
		return fmt.Errorf("song not found on spotify")
	}

	track := results.Tracks.Tracks[0]
	logger = logger.With(slog.String("track_id", string(track.ID)))

	logger.Info("found track on spotify",
		slog.String("name", track.Name),
		slog.String("artists", artistNames(track.Artists)))

	if playlistCache.Has(string(track.ID)) {
		logger.Info("track already in playlist")
		lastTitle = title
		return nil
	}

	logger.Debug("adding track to playlist")
	_, err = client.AddTracksToPlaylist(ctx, playlistID, track.ID)
	if err == nil {
		len := playlistCache.Add(string(track.ID))
		logger.Info("added track to playlist", slog.Int("length", len))
		lastTitle = title
		return nil
	}

	if errors.Is(err, context.Canceled) {
		slog.Warn("token expired, trying to refresh")

		return refreshToken(ctx)
	}

	return err
}

func getFullPlaylist(ctx context.Context, playlistID string) (*internal.Playlist, error) {
	playlist := internal.NewPlaylist([]string{})
	offset := 0

	for {
		playlistItems, err := client.GetPlaylistItems(ctx, spotify.ID(playlistID), spotify.Offset(offset))
		if err != nil {
			slog.Debug("error fetching playlist", slog.String("error", err.Error()))
			break
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

func refreshToken(ctx context.Context) error {
	tok, err := auth.RefreshToken(ctx, authToken)
	if err != nil {
		slog.Warn("refreshing token fail", slog.String("error", err.Error()))
		return err
	}

	slog.Debug("token refreshed", slog.Time("expires", tok.Expiry))

	// create new client with fresh token
	client = spotify.New(auth.Client(ctx, tok))

	return errRefreshedToken
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
		slog.Error("failed to parse token", slog.String("error", err.Error()))
		return
	}

	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		slog.Error("state mismatch", slog.String("expected", state), slog.String("got", st))
		return
	}

	slog.Debug("token granted", slog.Time("expires", authToken.Expiry))

	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), authToken))
	_, _ = fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
