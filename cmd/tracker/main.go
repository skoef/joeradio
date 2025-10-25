// Package main contains the runtime for the the Joe Radio tracker
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	"joeradio/internal"
)

const (
	websocketURL = "wss://socket.qmusic.be/api/502/ltfn4msd/websocket"
	playlistID   = "4t9w0OuAKt9mMEY27m1IDJ"
	redirectURI  = "http://localhost:8080/callback"
)

var (
	errRefreshedToken = errors.New("token was refreshed")
	errSongNotFound   = errors.New("song not found on spotify")

	auth *spotifyauth.Authenticator

	state string
	ch    = make(chan *spotify.Client)

	client        internal.SpotifyClient
	authToken     *oauth2.Token
	playlistCache *internal.Playlist
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
	fmt.Printf("Please log in to Spotify by visiting the following page in your browser:\n\n%s\n\n", url)

	// wait for auth to complete
	client = <-ch

	// keep a local cache of the playlist so we can easily check if a song is already in the playlist
	playlistCache, err = internal.GetFullPlaylist(ctx, client, playlistID)
	if err != nil {
		slog.Error("failed to fetch playlist", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Debug("loaded playlist", slog.Int("items", playlistCache.Len()))

	songs := make(chan *internal.Song)

	go func() {
		// close the channel if we're stopping this loop
		defer close(songs)

		handleWebsocket(ctx, songs)
	}()

	for song := range songs {
		if err := run(ctx, song); err != nil {
			logger.Warn("failed to complete run", slog.String("error", err.Error()))

			if errors.Is(err, errRefreshedToken) {
				logger.Debug("retrying immediately")

				if err := run(ctx, song); err != nil {
					logger.Warn("failed to complete run", slog.String("error", err.Error()))
				}
			}
		}
	}

	slog.Warn("main loop stopped")
}

// handleWebsocket opens a websocket, joins a channel and keeps reading messages
// if the websocket closes, it re-opens the websocket
func handleWebsocket(ctx context.Context, songs chan<- *internal.Song) {
	logger := slog.With("func", "handleWebsocket")

	// keep retrying the websocket
	for {
		c, _, err := websocket.DefaultDialer.DialContext(ctx, websocketURL, nil)
		if err != nil {
			logger.Error("failed to connect websocket", slog.String("error", err.Error()))
			return
		}

		defer func() {
			if err = c.Close(); err != nil {
				logger.Warn("could not close websocket", slog.String("error", err.Error()))
			}
		}()

		// keep reading messages
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				// check if the connection was closed
				var closeError *websocket.CloseError
				if errors.As(err, &closeError) {
					// when connection was closed, reopen after 10 seconds
					logger.Warn("connection was closed, reconnecting",
						slog.Int("code", closeError.Code),
						slog.String("error", closeError.Text))
					time.Sleep(time.Second * 10)
					break
				}

				// other issue
				logger.Error("failed to read message",
					slog.String("error", err.Error()))
				return
			}

			switch string(message) {
			case "o": // welcome message
				logger.Debug("received welcome")

				if err = c.WriteMessage(websocket.TextMessage, []byte(internal.JoinMessage)); err != nil {
					logger.Error("failed to join",
						slog.String("error", err.Error()))
					return
				}

			case "h": // heartbeat, ignore
			default:
				song, err := internal.ParseAMessage(message)
				if err != nil {
					logger.Error("failed to parse message",
						slog.String("error", err.Error()))
				}

				songs <- song
			}
		}
	}
}

func run(ctx context.Context, song *internal.Song) error {
	title := song.String()
	logger := slog.With(slog.String("title", title))

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
		return errSongNotFound
	}

	track := results.Tracks.Tracks[0]

	logger = logger.With(slog.String("track_id", string(track.ID)))

	logger.Info("found track on spotify",
		slog.String("name", track.Name),
		slog.String("artists", internal.ArtistNames(track.Artists)))

	if playlistCache.Has(string(track.ID)) {
		logger.Info("track already in playlist")

		return nil
	}

	logger.Debug("adding track to playlist")

	_, err = client.AddTracksToPlaylist(ctx, playlistID, track.ID)
	if err == nil {
		playlistLen := playlistCache.Add(string(track.ID))
		logger.Info("added track to playlist", slog.Int("length", playlistLen))

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
