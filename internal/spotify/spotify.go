package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/cybre/yeelight-controller/internal/config"
	"github.com/cybre/yeelight-controller/internal/errors"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.mills.io/bitcask/v2"
	"golang.org/x/oauth2"
)

var (
	auth          = spotifyauth.New(spotifyauth.WithRedirectURL(config.SpotifyRedirectURL), spotifyauth.WithClientID(config.SpotifyClientID), spotifyauth.WithClientSecret(config.SpotifyClientSecret), spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate, spotifyauth.ScopeUserReadCurrentlyPlaying, spotifyauth.ScopeUserReadPlaybackState))
	clientChannel = make(chan *spotify.Client)
	errChannel    = make(chan error)
	state         = "abc123"
	timeout       = 5 * time.Minute
)

// New creates a new Spotify client and handles authentication.
func New(ctx context.Context, db bitcask.DB, callbackPort uint16) (*spotify.Client, error) {
	client, err := getClientFromDB(ctx, db)
	if err != nil {
		return nil, errors.Wrapf(err, "get spotify client from DB")
	}

	if client != nil {
		return client, nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/spotify/callback", completeAuth)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", callbackPort),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	defer func() {
		server.Close()
		close(clientChannel)
		close(errChannel)
	}()

	url := auth.AuthURL(state)
	slog.Info("Please log in to Spotify by visiting the following page in your browser", slog.String("url", url))

	select {
	case <-ctx.Done():
		return nil, errors.Wrapf(ctx.Err(), "spotify authentication")
	case <-time.After(timeout):
		return nil, errors.Errorf("spotify authentication timeout")
	case client = <-clientChannel:
		token, err := client.Token()
		if err != nil {
			return nil, errors.Wrapf(err, "get spotify token from client")
		}

		buf, err := json.Marshal(token)
		if err != nil {
			return nil, errors.Wrapf(err, "json marshal spotify token")
		}

		if err := db.Put([]byte("spotifyToken"), buf); err != nil {
			return nil, errors.Wrapf(err, "put spotify token json")
		}

		return client, nil
	case err := <-errChannel:
		return nil, errors.Wrapf(err, "spotify authentication")
	}
}

func getClientFromDB(ctx context.Context, db bitcask.DB) (*spotify.Client, error) {
	tokenBytes, err := db.Get([]byte("spotifyToken"))
	if err != nil {
		if err != bitcask.ErrKeyNotFound {
			return nil, errors.Wrapf(err, "get spotify token from DB")
		}

		return nil, nil
	}

	var token oauth2.Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, errors.Wrapf(err, "unmarshal spotify token")
	}

	return spotify.New(auth.Client(ctx, &token)), nil

}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		errChannel <- errors.Wrap(err)
		return
	}

	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		errChannel <- errors.Errorf("state mismatch: %s != %s", st, state)
		return
	}

	client := spotify.New(auth.Client(r.Context(), tok))
	fmt.Fprintf(w, "Login Completed!")

	clientChannel <- client
}
