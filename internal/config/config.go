package config

import (
	"flag"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

var (
	// SpotifyRedirectURL
	SpotifyRedirectURL string
	// SpotifyCallbackPort is the port to listen on for the Spotify callback
	SpotifyCallbackPort uint16
	// SpotifyClientID is the Spotify client ID
	SpotifyClientID string
	// SpotifyClientSecret is the Spotify client secret
	SpotifyClientSecret string
	// Debug is a flag to enable debug logging
	Debug bool
	// MusicModePort is the port to listen on for music mode
	MusicModePort uint16
)

func init() {
	_ = godotenv.Load()

	port, err := strconv.ParseUint(os.Getenv("SPOTIFY_CALLBACK_PORT"), 10, 16)
	if err != nil {
		panic(err)
	}
	SpotifyCallbackPort = uint16(port)

	SpotifyRedirectURL = os.Getenv("SPOTIFY_REDIRECT_URL")
	SpotifyClientID = os.Getenv("SPOTIFY_ID")
	SpotifyClientSecret = os.Getenv("SPOTIFY_SECRET")

	port, err = strconv.ParseUint(os.Getenv("MUSIC_MODE_PORT"), 10, 16)
	if err != nil {
		panic(err)
	}
	MusicModePort = uint16(port)

	debugFlag := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	Debug = *debugFlag || os.Getenv("DEBUG") == "true"
}
