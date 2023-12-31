package yeelight

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/cybre/yeelight-controller/internal/errors"
)

type commandError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *commandError) Error() string {
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

type commandResult struct {
	ID     int           `json:"id"`
	Result []string      `json:"result"`
	Error  *commandError `json:"error"`
}

type notification struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

type Bulb struct {
	bulbBase

	results            chan commandResult
	musicContextCancel context.CancelFunc
}

func newBulb(addr netip.AddrPort) Bulb {
	results := make(chan commandResult)

	return Bulb{
		bulbBase: bulbBase{
			bulbInfo: &bulbInfo{
				addr: addr,
			},
			commandCallback: getCommandExecutionCallback(results),
		},
		results: results,
	}
}

func (bb *Bulb) Connect(ctx context.Context) error {
	conn, err := net.Dial("tcp", bb.Addr().String())
	if err != nil {
		return errors.Wrapf(err, "connect to bulb")
	}

	bb.conn = conn

	bb.listen(ctx)

	return nil
}

func (bb *Bulb) EnableMusicMode(ctx context.Context, port uint16, callback func(context.Context, *MusicModeBulb) error) error {
	localAddr := bb.conn.LocalAddr()
	if localAddr == nil {
		return errors.New("bulb is not connected")
	}

	splitAddr := strings.Split(localAddr.String(), ":")

	if len(splitAddr) != 2 {
		return errors.New("invalid local address")
	}

	ip := splitAddr[0]

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return errors.Wrapf(err, "start music mode listener")
	}
	defer ln.Close()

	if _, err = bb.executeCommand(ctx, "set_music", 1, ip, port); err != nil {
		return errors.Wrapf(err, "enable music mode")
	}

	conn, err := ln.Accept()
	if err != nil {
		return errors.Wrapf(err, "accept connection from bulb")
	}

	musicContext, musicContextCancel := context.WithCancel(ctx)
	bb.musicContextCancel = musicContextCancel

	bulb := newMusicModeBulb(bb.bulbInfo, conn)
	defer func() {
		if bb.musicContextCancel != nil {
			bb.musicContextCancel()
			bb.musicContextCancel = nil
		}
		if err := bulb.Disconnect(); err != nil {
			slog.Error("disconnect bulb in music mode", slog.Any("error", err))
		}
	}()

	err = callback(musicContext, bulb)

	return errors.Wrapf(err, "music mode callback")
}

func (bb *Bulb) DisableMusicMode(ctx context.Context) error {
	if bb.musicContextCancel != nil {
		bb.musicContextCancel()
		bb.musicContextCancel = nil
	}

	_, err := bb.executeCommand(ctx, "set_music", 0)

	return err
}

func (bb *Bulb) listen(ctx context.Context) {
	// Poll for props
	go func() {
		for {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				res, err := bb.executeCommand(ctx, "get_prop", "power", "bright", "color_mode", "ct", "rgb", "hue", "sat", "name")
				if err != nil {
					slog.Error("get bulb props", slog.Any("error", err))
					continue
				}

				for i, prop := range res {
					switch i {
					case 0:
						bb.power = PowerStatus(prop)
					case 1:
						brightnessInt, err := strconv.ParseUint(prop, 10, 8)
						if err != nil {
							slog.Warn("failed to convert brightness to int", slog.Any("error", err))
							continue
						}

						bb.brightness = uint8(brightnessInt)
					case 2:
						colorModeInt, err := strconv.ParseUint(prop, 10, 8)
						if err != nil {
							slog.Warn("failed to convert color mode to int", slog.Any("error", err))
							continue
						}

						bb.colorMode = ColorMode(colorModeInt)
					case 3:
						colorTemperatureInt, err := strconv.ParseUint(prop, 10, 16)
						if err != nil {
							slog.Warn("failed to convert color temperature to int", slog.Any("error", err))
							continue
						}

						bb.colorTemperature = uint16(colorTemperatureInt)
					case 4:
						rgbInt, err := strconv.ParseUint(prop, 10, 32)
						if err != nil {
							slog.Warn("failed to convert RGB to int", slog.Any("error", err))
							continue
						}

						bb.rgb = uint(rgbInt)
					case 5:
						hueInt, err := strconv.ParseUint(prop, 10, 16)
						if err != nil {
							slog.Warn("failed to convert hue to int", slog.Any("error", err))
							continue
						}

						bb.hue = uint16(hueInt)
					case 6:
						saturationInt, err := strconv.ParseUint(prop, 10, 8)
						if err != nil {
							slog.Warn("failed to convert saturation to int", slog.Any("error", err))
							continue
						}

						bb.saturation = uint8(saturationInt)
					case 7:
						bb.name = prop
					}
				}
			}
		}
	}()

	// Listen for prop updates and results
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			buf := make([]byte, 1024)
			n, err := bb.conn.Read(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}

				panic(err)
			}

			// parse response
			resp := string(buf[:n])
			slog.Debug("received response from bulb", slog.String("response", resp))
			lines := strings.Split(resp, lineEnding)

			// parse each line
			for _, line := range lines {
				if strings.HasPrefix(line, "{\"id\":") {
					var result commandResult
					if err := json.Unmarshal([]byte(line), &result); err != nil {
						slog.Error("failed to unmarshal result", slog.String("json", line), slog.Any("error", err))
						continue
					}

					bb.results <- result
				}

				if strings.HasPrefix(line, "{\"method\":") {
					var notification notification
					if err := json.Unmarshal([]byte(line), &notification); err != nil {
						slog.Error("failed to unmarshal notification", slog.String("json", line), slog.Any("error", err))
						continue
					}

					switch notification.Method {
					case "props":
						for key, value := range notification.Params {
							switch key {
							case "power":
								bb.power = PowerStatus(value.(string))
							case "bright":
								bb.brightness = uint8(value.(float64))
							case "color_mode":
								bb.colorMode = ColorMode(int(value.(float64)))
							case "ct":
								bb.colorTemperature = uint16(value.(float64))
							case "rgb":
								bb.rgb = uint(value.(float64))
							case "hue":
								bb.hue = uint16(value.(float64))
							case "sat":
								bb.saturation = uint8(value.(float64))
							case "name":
								bb.name = value.(string)
							}
						}
					}
				}
			}
		}
	}()
}

func getCommandExecutionCallback(results <-chan commandResult) func(context.Context, command) ([]string, error) {
	return func(ctx context.Context, cmd command) ([]string, error) {
		select {
		case result := <-results:
			if result.ID == cmd.ID {
				if result.Error != nil {
					return nil, errors.Wrapf(result.Error, "%s (%v)", cmd.Method, cmd.Params)
				}

				if len(result.Result) == 1 && result.Result[0] == "ok" {
					return nil, nil
				}

				return result.Result, nil
			}
		case <-time.After(timeout):
			return nil, errors.New("command timed out")
		case <-ctx.Done():
			return nil, errors.Wrapf(ctx.Err(), "execute command %s (%v)", cmd.Method, cmd.Params)
		}

		return nil, errors.New("get command result")
	}
}
