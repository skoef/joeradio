package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	"joeradio/provider"
)

const (
	tokenFilename = "spotify.token"
	redirectURI   = "http://127.0.0.1:8080/callback"
)

// Client describes the functions we use on the spotify.Client so we can
// mock them for testing
type Client interface {
	Search(ctx context.Context, query string, t spotify.SearchType, opts ...spotify.RequestOption) (*spotify.SearchResult, error)
	AddTracksToPlaylist(ctx context.Context, playlistID spotify.ID, trackIDs ...spotify.ID) (snapshotID string, err error)
	GetPlaylistItems(ctx context.Context, playlistID spotify.ID, opts ...spotify.RequestOption) (*spotify.PlaylistItemPage, error)
}

// Spotify implements the Provider interface
type Spotify struct {
	client        Client
	authenticator *spotifyauth.Authenticator
	authState     string
	token         *oauth2.Token
	ch            chan *oauth2.Token
	tokenPath     string
	logger        *slog.Logger
	playlistID    spotify.ID
}

// New returns new Spotify provider
func New(config provider.Config) (*Spotify, error) {
	return &Spotify{
		logger:     config.Logger.With(slog.String("provider", "spotify")),
		tokenPath:  config.SpotifyTokenPath,
		playlistID: spotify.ID(config.SpotifyPlaylistID),
		authenticator: spotifyauth.New(
			spotifyauth.WithRedirectURL(redirectURI),
			spotifyauth.WithClientID(config.SpotifyClientID),
			spotifyauth.WithClientSecret(config.SpotifyClientSecret),
			spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic),
		),
	}, nil
}

// Name returns the provider's name
func (Spotify) Name() string {
	return "spotify"
}

// Authenticate tries to find a reusable token or starts new authentication process and returns a new token
// The path
func (s *Spotify) Authenticate(ctx context.Context) error {
	var token oauth2.Token

	// try to get a token from file first
	s.logger.Debug("trying to re-use existing token")

	filename := filepath.Join(s.tokenPath, tokenFilename)

	//nolint:gosec // G304, this is intended functionality here
	data, err := os.ReadFile(filename)
	if err == nil {
		s.logger.Debug("found existing token", slog.String("filename", filename))

		if err = json.Unmarshal(data, &token); err != nil {
			return fmt.Errorf("could not unmarshal token: %w", err)
		}
	}

	// check if token is expired
	if time.Since(token.Expiry) >= 0 {
		s.logger.Info("token expired, starting authentication process")

		freshToken, err := s.webAuth(ctx)
		if err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		if err := s.cacheToken(freshToken); err != nil {
			return fmt.Errorf("could not cache token: %w", err)
		}
	}

	s.logger.Info("authentication complete", slog.Time("expiry", token.Expiry))

	s.client = spotify.New(s.authenticator.Client(ctx, &token))
	s.token = &token

	return nil
}

// GetFullPlaylist returns the playlist
func (s *Spotify) GetFullPlaylist(ctx context.Context) (*provider.Playlist, error) {
	playlist := provider.NewPlaylist()
	offset := 0

	s.logger.Debug("fetching playlist", slog.String("playlist", string(s.playlistID)))

	for {
		playlistItems, err := s.client.GetPlaylistItems(ctx, s.playlistID, spotify.Offset(offset))
		if err != nil {
			return nil, err
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

// Search performs query and returns the results
func (s *Spotify) Search(ctx context.Context, query string) (provider.Track, error) {
	// limit search to the range of 1970 until 1999, since Joe is a station dedicated
	// to 70's, 80's and 90's music
	// this prevents us from getting remixes from later years in the results
	results, err := s.client.Search(ctx, query+" year:1970-1999", spotify.SearchTypeTrack, spotify.Limit(1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, s.refreshToken(ctx)
		}
		return nil, fmt.Errorf("could not search: %w", err)
	}

	if len(results.Tracks.Tracks) == 0 {
		return nil, provider.ErrSongNotFound
	}

	return newTrack(results.Tracks.Tracks[0]), nil
}

// AddToPlaylist adds given track to playlist
func (s *Spotify) AddToPlaylist(ctx context.Context, trackID string) error {
	_, err := s.client.AddTracksToPlaylist(ctx, s.playlistID, spotify.ID(trackID))
	if err != nil && errors.Is(err, context.Canceled) {
		return s.refreshToken(ctx)
	}

	return err
}

func (s *Spotify) cacheToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("could not marshal token: %w", err)
	}

	filename := filepath.Join(s.tokenPath, tokenFilename)
	if err = os.WriteFile(filename, data, 0o600); err != nil {
		return fmt.Errorf("could not write token to file: %w", err)
	}

	return nil
}

func (s *Spotify) refreshToken(ctx context.Context) error {
	s.logger.Warn("token expired, trying to refresh")

	freshToken, err := s.authenticator.RefreshToken(ctx, s.token)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if err = s.cacheToken(freshToken); err != nil {
		s.logger.Warn("could not cache token", slog.String("error", err.Error()))
	}

	// create new client with fresh token
	s.client = spotify.New(s.authenticator.Client(ctx, freshToken))
	s.token = freshToken

	return provider.ErrRefreshedToken
}

func (s *Spotify) webAuth(ctx context.Context) (*oauth2.Token, error) {
	// seed RNG and create a state for authentication
	rand.New(rand.NewSource(time.Now().UnixNano()))

	s.authState = strconv.Itoa(rand.Int())

	// prepare channel for returning token
	s.ch = make(chan *oauth2.Token)

	// create a new context which we can cancel if something goes wrong inside
	// the routine for the http server
	cancelCtx, cancel := context.WithCancel(ctx)

	var httpServer *http.Server

	go func() {
		httpServer = &http.Server{
			Addr:         ":8080",
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Handler:      http.HandlerFunc(s.handleCallback),
		}
		if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			cancel()
		}
	}()

	url := s.authenticator.AuthURL(s.authState)
	fmt.Printf("Please log in to Spotify by visiting the following page in your browser:\n\n%s\n\n", url)

	// wait for auth to complete or context to be canceled
	select {
	case <-cancelCtx.Done():
		return nil, cancelCtx.Err()
	case token := <-s.ch:
		return token, nil
	}
}

func (s *Spotify) handleCallback(w http.ResponseWriter, r *http.Request) {
	var err error

	token, err := s.authenticator.Token(r.Context(), s.authState, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		s.logger.Error("failed to parse token", slog.String("error", err.Error()))

		return
	}

	if st := r.FormValue("state"); st != s.authState {
		http.NotFound(w, r)
		s.logger.Error("state mismatch",
			slog.String("expected", s.authState),
			slog.String("got", st),
		)

		return
	}

	s.logger.Debug("token granted", slog.Time("expires", token.Expiry))

	// let the client know authentication completed
	_, _ = fmt.Fprintf(w, "Login Completed! You can close this window now!")

	// send token on authentication channel what authenticate() is waiting for
	s.ch <- token
}
