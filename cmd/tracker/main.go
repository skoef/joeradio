// Package main contains the runtime for the the Joe Radio tracker
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	"joeradio/internal"
)

const (
	streamURL   = "https://icecast-qmusicnl-cdp.triple-it.nl/Joe_nl_high.aac"
	playlistID  = "4t9w0OuAKt9mMEY27m1IDJ"
	redirectURI = "http://localhost:8080/callback"
)

var (
	errRefreshedToken   = errors.New("token was refreshed")
	errCurrentlyNoTitle = errors.New("currently nothing is playing")
	errSongNotFound     = errors.New("song not found on spotify")

	auth *spotifyauth.Authenticator

	state string
	ch    = make(chan *spotify.Client)

	ignoreTitles = []string{
		"JOE nieuws",
		"Ad break",
	}

	client        internal.SpotifyClient
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

	// try to load .env
	err := godotenv.Load()
	if err != nil {
		slog.Warn("env not loaded", slog.String("error", err.Error()))
	}

	// set up authenticator
	// we will use that once for getting a token and then afterwards for refreshing
	// the existing token
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")

	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		logger.Error("set both SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables")
		os.Exit(1)
	}

	auth = spotifyauth.New(spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithClientID(clientID),
		spotifyauth.WithClientSecret(clientSecret),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic))

	// first start an HTTP server
	http.HandleFunc("/callback", completeAuth)

	go func() {
		//nolint:gosec // this is a very short-lived webserver
		// TODO: create separate http server and close this when auth happened
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

	playlistCache, err = internal.GetFullPlaylist(ctx, client, playlistID)
	if err != nil {
		slog.Error("failed to fetch playlist", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Debug("loaded playlist", slog.Int("items", playlistCache.Len()))
	logger.Debug("start tracking icecast", slog.String("url", streamURL))

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
	title, err := GetStreamTitle(ctx, streamURL)
	if err != nil {
		return err
	}

	if title == "" {
		return errCurrentlyNoTitle
	}

	logger := slog.With(slog.String("title", title))

	// should we ignore this title?
	if slices.Contains(ignoreTitles, title) {
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
	results, err := client.Search(ctx, title+" year:1970-1999", spotify.SearchTypeTrack, spotify.Limit(1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Warn("token expired, trying to refresh")

			return refreshToken(ctx)
		}

		return fmt.Errorf("could not search: %w", err)
	}

	if len(results.Tracks.Tracks) == 0 {
		lastTitle = title
		return errSongNotFound
	}

	track := results.Tracks.Tracks[0]

	logger = logger.With(slog.String("track_id", string(track.ID)))

	logger.Info("found track on spotify",
		slog.String("name", track.Name),
		slog.String("artists", internal.ArtistNames(track.Artists)))

	if playlistCache.Has(string(track.ID)) {
		logger.Info("track already in playlist")

		lastTitle = title

		return nil
	}

	logger.Debug("adding track to playlist")

	_, err = client.AddTracksToPlaylist(ctx, playlistID, track.ID)
	if err == nil {
		playlistLen := playlistCache.Add(string(track.ID))
		logger.Info("added track to playlist", slog.Int("length", playlistLen))

		lastTitle = title

		return nil
	}

	if errors.Is(err, context.Canceled) {
		slog.Warn("token expired, trying to refresh")

		return refreshToken(ctx)
	}

	return err
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

func GetStreamTitle(ctx context.Context, streamURL string) (string, error) {
	m, err := getStreamMetas(ctx, streamURL)
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

func getStreamMetas(ctx context.Context, streamURL string) ([]byte, error) {
	client := &http.Client{}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, http.NoBody)
	req.Header.Set("Icy-Metadata", "1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("could not close body", slog.String("error", err.Error()))
		}
	}()

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
