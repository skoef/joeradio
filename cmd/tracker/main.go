// Package main contains the runtime for the Joe Radio tracker
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"

	"joeradio/provider"
	"joeradio/provider/spotify"
	"joeradio/source"
)

var (
	prov          provider.Provider
	playlistCache *provider.Playlist
)

func main() {
	if err := mainE(); err != nil {
		slog.Error("runtime error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func mainE() error {
	// try to load .env but fail silently when it's not found
	err := godotenv.Load()
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load .env: %w", err)
		}
	}

	config := provider.NewDefaultConfig()
	flag.StringVar(&config.SpotifyTokenPath, "spotify-token-path", config.SpotifyTokenPath, "path for caching spotify authentication token")
	flag.StringVar(&config.SpotifyClientID, "spotify-client-id", os.Getenv("SPOTIFY_CLIENT_ID"), "spotify client ID")
	flag.StringVar(&config.SpotifyClientSecret, "spotify-client-secret", os.Getenv("SPOTIFY_CLIENT_SECRET"), "spotify client secret")
	flag.StringVar(&config.SpotifyPlaylistID, "spotify-playlist-id", os.Getenv("SPOTIFY_PLAYLIST_ID"), "spotify playlist ID")
	flag.StringVar(&config.Provider, "provider", config.Provider, "choose provider, currently only spotify")
	flag.BoolVar(&config.Debug, "debug", config.Debug, "enable debug logging")

	flag.Parse()

	// set up logging
	logOpts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if config.Debug {
		logOpts.Level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, logOpts))
	slog.SetDefault(logger)

	config.Logger = logger

	// validate configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	ctx := context.Background()

	// create spotify provider
	prov, err = spotify.New(config)
	if err != nil {
		return fmt.Errorf("failed to setup %s provider: %w", prov.Name(), err)
	}

	if err := prov.Authenticate(ctx); err != nil {
		return fmt.Errorf("failed to authenticate %s provider: %w", prov.Name(), err)
	}

	// keep a local cache of the playlist so we can easily check if a song is already in the playlist
	playlistCache, err = prov.GetFullPlaylist(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch playlist: %w", err)
	}

	logger.Debug("loaded playlist", slog.Int("items", playlistCache.Len()))

	songs := make(chan provider.Track)

	go func() {
		// close the channel if we're stopping this loop
		defer close(songs)

		handleWebsocket(ctx, songs)
	}()

	for song := range songs {
		if err := run(ctx, song); err != nil {
			logger.Warn("failed to complete run", slog.String("error", err.Error()))

			if errors.Is(err, provider.ErrRefreshedToken) {
				logger.Debug("retrying immediately")

				if err := run(ctx, song); err != nil {
					logger.Warn("failed to complete retried run", slog.String("error", err.Error()))
				}
			}
		}
	}

	logger.Warn("main loop stopped")

	return nil
}

// handleWebsocket opens a websocket, joins a channel and keeps reading messages
// if the websocket closes, it re-opens the websocket
func handleWebsocket(ctx context.Context, songs chan<- provider.Track) {
	logger := slog.With("func", "handleWebsocket")

	// keep retrying the websocket
	for {
		c, _, err := websocket.DefaultDialer.DialContext(ctx, source.WebsocketURL, nil)
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
				} else {
					// other issue
					logger.Error("failed to read message",
						slog.String("error", err.Error()))
				}

				time.Sleep(time.Second * 10)

				break
			}

			switch string(message) {
			case "o": // welcome message
				logger.Debug("received welcome")

				if err = c.WriteMessage(websocket.TextMessage, []byte(source.JoinMessage)); err != nil {
					logger.Error("failed to join",
						slog.String("error", err.Error()))

					return
				}

			case "h": // heartbeat, ignore
			default:
				song, err := source.ParseAMessage(message)
				if err != nil {
					logger.Error("failed to parse message",
						slog.String("error", err.Error()))
				}

				songs <- song
			}
		}
	}
}

func run(ctx context.Context, song provider.Track) error {
	title := song.GetTitle()
	logger := slog.With(
		slog.String("title", title),
		slog.String("provider", prov.Name()),
	)

	logger.Info("search title")
	track, err := prov.Search(ctx, title)
	if err != nil {
		if errors.Is(err, provider.ErrSongNotFound) {
			return provider.ErrSongNotFound
		}

		return fmt.Errorf("could not search: %w", err)
	}

	logger = logger.With(slog.String("track_id", track.GetID()))

	logger.Info("found track",
		slog.String("title", track.GetTitle()),
		slog.String("artists", strings.Join(track.GetArtists(), ",")))

	if playlistCache.Has(track.GetID()) {
		logger.Info("track already in playlist")

		return nil
	}

	logger.Debug("adding track to playlist")

	err = prov.AddToPlaylist(ctx, track.GetID())
	if err == nil {
		playlistLen := playlistCache.Add(track.GetID())
		logger.Info("added track to playlist", slog.Int("length", playlistLen))

		return nil
	}

	return err
}
