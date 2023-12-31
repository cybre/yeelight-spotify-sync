package main

import (
	"context"
	"log"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"

	"github.com/cybre/yeelight-controller/internal/config"
	"github.com/cybre/yeelight-controller/internal/errors"
	"github.com/cybre/yeelight-controller/internal/homekit"
	spotifyinternal "github.com/cybre/yeelight-controller/internal/spotify"
	"github.com/cybre/yeelight-controller/internal/utils"
	"github.com/cybre/yeelight-controller/internal/yeelight"
	goerrors "github.com/go-errors/errors"
	"github.com/zmb3/spotify/v2"
	"go.mills.io/bitcask/v2"
	"golang.org/x/sync/errgroup"
)

const (
	frameRate = 60
)

type lightshowState struct {
	playerState *spotify.PlayerState
	cancel      context.CancelFunc
}

var spotifyCache = struct {
	features map[spotify.ID]spotify.AudioFeatures
	analysis map[spotify.ID]spotify.AudioAnalysis
}{
	features: make(map[spotify.ID]spotify.AudioFeatures),
	analysis: make(map[spotify.ID]spotify.AudioAnalysis),
}

var playMutex sync.Mutex

var brightnessModifier = 1.0

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var loggerOpts *slog.HandlerOptions = nil
	if config.Debug {
		loggerOpts = &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, loggerOpts))
	slog.SetDefault(logger)

	db, err := bitcask.Open("./database")
	if err != nil {
		slog.Error("failed to open bitcask database", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()

	spotifyClient, err := getSpotifyClient(ctx, db)
	if err != nil {
		slog.Error("failed to get spotify client", slog.String("stack", err.(*goerrors.Error).ErrorStack()))
		os.Exit(1)
	}

	bulb, err := getBulb(ctx)
	if err != nil {
		slog.Error("failed to get bulb", slog.String("stack", err.(*goerrors.Error).ErrorStack()))
		os.Exit(1)
	}
	if bulb == nil {
		slog.Error("no bulb found")
		os.Exit(1)
	}

	defer func() {
		if err := bulb.Disconnect(); err != nil {
			slog.Warn("failed to disconnect from bulb", slog.Any("error", err))
		}
	}()

	if err := homekit.SetUp(ctx, int(brightnessModifier*100), true, func(power bool) {
		var err error
		if power {
			err = bulb.TurnOn(ctx, yeelight.Smooth, 500)
		} else {
			err = bulb.TurnOff(ctx, yeelight.Smooth, 500)
		}
		if err != nil {
			slog.Error("failed to set power via homekit", slog.Bool("power", power), slog.Any("error", err))
		}
	}, func(brightness int) {
		brightnessModifier = float64(brightness) / 100.0
	}); err != nil {
		slog.Error("failed to set up homekit", slog.String("stack", err.(*goerrors.Error).ErrorStack()))
	}

	if err := bulb.EnableMusicMode(ctx, config.MusicModePort, func(ctx context.Context, bulb *yeelight.MusicModeBulb) error {
		spotifyTicker := time.NewTicker(1 * time.Second)
		defer spotifyTicker.Stop()

		var state *lightshowState

		for {
			select {
			case <-spotifyTicker.C:
				if bulb.Power() == yeelight.PowerOff {
					if state != nil {
						state.cancel()
						state = nil
					}
					continue
				}

				playerState, err := spotifyClient.PlayerState(ctx)
				if err != nil {
					slog.Error("get player state", slog.Any("error", err))
					continue
				}

				if playerState.Item == nil || !playerState.Playing {
					if state != nil {
						state.cancel()
						state = nil
					}
					continue
				}

				if state == nil || playerState.Item.ID != state.playerState.Item.ID {
					if state != nil {
						state.cancel()
					}

					trackCtx, cancelTrack := context.WithCancel(ctx)
					state = &lightshowState{
						playerState: playerState,
						cancel:      cancelTrack,
					}

					go func() {
						if err := startTrackSync(trackCtx, spotifyClient, playerState, bulb); err != nil {
							slog.Error("start track sync", slog.String("stack", err.(*goerrors.Error).ErrorStack()))
						}
					}()
				} else {
					state.playerState.Progress = playerState.Progress - 300
				}
			case <-ctx.Done():
				spotifyTicker.Stop()
				return nil
			}
		}
	}); err != nil {
		log.Fatal(err.(*goerrors.Error).ErrorStack())
	}
}

func startTrackSync(ctx context.Context, spotifyClient *spotify.Client, playerState *spotify.PlayerState, bulb *yeelight.MusicModeBulb) error {
	var audioFeatures *spotify.AudioFeatures
	var audioAnalysis *spotify.AudioAnalysis

	errGroup, groupCtx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		if cachedAudioFeatures, ok := spotifyCache.features[playerState.Item.ID]; ok {
			audioFeatures = &cachedAudioFeatures
			return nil
		}

		var err error

		features, err := spotifyClient.GetAudioFeatures(groupCtx, playerState.Item.ID)
		if err != nil {
			return errors.Wrapf(err, "get audio features")
		}

		audioFeatures = features[0]

		spotifyCache.features[playerState.Item.ID] = *features[0]

		return nil

	})
	errGroup.Go(func() error {
		if cachedAudioAnalysis, ok := spotifyCache.analysis[playerState.Item.ID]; ok {
			audioAnalysis = &cachedAudioAnalysis
			return nil
		}

		var err error

		analysis, err := spotifyClient.GetAudioAnalysis(groupCtx, playerState.Item.ID)
		if err != nil {
			return errors.Wrapf(err, "get audio analysis")
		}

		audioAnalysis = analysis

		spotifyCache.analysis[playerState.Item.ID] = *analysis

		return nil
	})

	if err := errGroup.Wait(); err != nil {
		return errors.Wrap(err)
	}

	if audioFeatures == nil || audioAnalysis == nil {
		return errors.New("missing audio features or analysis")
	}

	playMutex.Lock()
	defer playMutex.Unlock()

	if err := lightShow(ctx, playerState, audioFeatures, audioAnalysis, bulb); err != nil {
		return errors.Wrapf(err, "light show")
	}

	return nil
}

func lightShow(ctx context.Context, playerState *spotify.PlayerState, audioFeatures *spotify.AudioFeatures, audioAnalysis *spotify.AudioAnalysis, bulb *yeelight.MusicModeBulb) error {
	// Start a ticker to update the progress
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if playerState.Progress >= playerState.Item.Duration {
					return
				}
				playerState.Progress += 10
			}
		}
	}()

	allLoudnesses := utils.Map(audioAnalysis.Segments, func(s spotify.Segment) float64 {
		return s.LoudnessMax
	})

	averageTrackLoudness := utils.Avg(allLoudnesses)

	type trackLoudnesses struct {
		highest float64
		lowest  float64
	}
	relativeTrackLoudnesses := utils.Reduce(allLoudnesses, func(acc trackLoudnesses, loudness float64) trackLoudnesses {
		relativeLoudness := calculateNormalizedSegmentLoudness(loudness, averageTrackLoudness)
		if relativeLoudness < acc.lowest {
			acc.lowest = relativeLoudness
		}
		if relativeLoudness > acc.highest {
			acc.highest = relativeLoudness
		}

		return acc
	}, trackLoudnesses{
		highest: math.Inf(-1),
		lowest:  math.Inf(1),
	})

	previousBarIdx := -1
	previousHue, previousSaturation, previousBrightness := 0.0, 0.0, 0.0
	hue, saturation := 0.0, 0.0

	for playerState.Progress < playerState.Item.Duration {
		if bulb.Power() == yeelight.PowerOff {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		default:
		}

		currentSectionIdx := slices.IndexFunc(audioAnalysis.Sections, func(s spotify.Section) bool {
			return playerState.Progress >= int(s.Start*1000) && playerState.Progress < int((s.Start+s.Duration)*1000)
		})

		currentBarIdx := slices.IndexFunc(audioAnalysis.Bars, func(s spotify.Marker) bool {
			return playerState.Progress >= int(s.Start*1000) && playerState.Progress < int((s.Start+s.Duration)*1000)
		})

		currentSegmentIdx := slices.IndexFunc(audioAnalysis.Segments, func(s spotify.Segment) bool {
			return playerState.Progress >= int(s.Start*1000) && playerState.Progress < int((s.Start+s.Duration)*1000)
		})

		if currentSectionIdx == -1 || currentBarIdx == -1 || currentSegmentIdx == -1 {
			time.Sleep(time.Duration(1000/frameRate) * time.Millisecond)
			continue
		}

		bar := audioAnalysis.Bars[currentBarIdx]
		segment := audioAnalysis.Segments[currentSegmentIdx]

		// Update hue for entire bars only
		if currentBarIdx != previousBarIdx {
			barSegments := utils.FilterFunc(audioAnalysis.Segments, func(s spotify.Segment) bool {
				return s.Start >= bar.Start && s.Start < bar.Start+bar.Duration
			})

			if len(barSegments) == 0 {
				continue
			}

			barSegmentLoudnesses := utils.Map(barSegments, func(s spotify.Segment) float64 {
				return s.LoudnessMax
			})
			maxBarLoudness := slices.Max(barSegmentLoudnesses)

			scaledLoudness := calculateNormalizedSegmentLoudness(maxBarLoudness, averageTrackLoudness)
			scale := utils.MapValue(scaledLoudness, relativeTrackLoudnesses.lowest, relativeTrackLoudnesses.highest, 0.0, 1.0)

			hue = 30 + scale*330
			hue = math.Mod(hue, 360)
			if hue < 0 {
				hue += 360
			}

			saturation = 40 + (scale * 60)

			previousHue = hue
			previousSaturation = saturation
			previousBarIdx = currentBarIdx
		}

		scaledLoudness := calculateNormalizedSegmentLoudness(segment.LoudnessMax, averageTrackLoudness)
		scale := utils.MapValue(scaledLoudness, relativeTrackLoudnesses.lowest, relativeTrackLoudnesses.highest, 0.0, 1.0)
		brightness := (40 + scale*60) * brightnessModifier

		if hue != previousHue || saturation != previousSaturation || brightness != previousBrightness {
			if err := bulb.SetHSV(ctx, uint16(hue), uint8(saturation), uint8(brightness), yeelight.Smooth, 100); err != nil {
				return err
			}
		}

		slog.Debug("frame", slog.String("progress", (time.Duration(playerState.Progress)*time.Millisecond).String()), slog.Int("hue", int(hue)), slog.Int("saturation", int(saturation)), slog.Int("brightness", int(brightness)))

		previousBrightness = brightness

		time.Sleep(time.Duration(1000/frameRate) * time.Millisecond)
	}

	return nil
}

func getBulb(ctx context.Context) (*yeelight.Bulb, error) {
	bulbs, err := yeelight.Discover(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "bulb discovery")
	}

	if len(bulbs) == 0 {
		return nil, nil
	}

	bulb := &bulbs[0]

	if err := bulb.Connect(ctx); err != nil {
		return nil, err
	}

	if err := bulb.TurnOn(ctx, yeelight.Smooth, 500); err != nil {
		return nil, err
	}

	if err := bulb.DisableMusicMode(ctx); err != nil {
		slog.Warn("disable music mode (probably not active)", slog.Any("error", err))
	}

	return bulb, nil
}

// Returns a loudness coefficient of a segment relative to the overall loudness of the track
func calculateNormalizedSegmentLoudness(segmentLoudnessMax, overallLoudness float64) float64 {
	relativeLoudness := segmentLoudnessMax - overallLoudness

	// Since dB is a logarithmic unit, translate the dB difference into a linear scale by raising 10 to the power.
	linearScaleLoudness := math.Pow(10, relativeLoudness/20) // Division by 20 to convert dB to linear scale

	// Now we have a linear scale factor representing how much louder or quieter the segment is compared to the overall loudness.
	// We'll normalize this scale to a range between 0 and 1.
	// To avoid division by zero, we set a lower limit for the overall loudness (-60 dB).
	minLinearScale := math.Pow(10, -60.0/20)
	maxLinearScale := 1.0
	normalizedLoudness := (linearScaleLoudness - minLinearScale) / (maxLinearScale - minLinearScale)

	return normalizedLoudness
}

func getSpotifyClient(ctx context.Context, db bitcask.DB) (*spotify.Client, error) {
	spotifyClient, err := spotifyinternal.New(ctx, db, config.SpotifyCallbackPort)
	if err != nil {
		return nil, errors.Wrapf(err, "create spotify client")
	}

	return spotifyClient, nil
}
