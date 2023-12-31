package homekit

import (
	"context"
	"log/slog"

	"github.com/brutella/hap"
	hapaccessory "github.com/brutella/hap/accessory"
	"github.com/cybre/yeelight-controller/internal/errors"
	"github.com/cybre/yeelight-controller/internal/homekit/accessory"
)

func SetUp(ctx context.Context, intialBrightness int, isOn bool, powerCallback func(bool), brightnessCallback func(int)) error {
	a := accessory.NewLightbulb(hapaccessory.Info{
		Name:         "Spotify LED Strip",
		SerialNumber: "0000002",
		Manufacturer: "Stefan Ric",
		Model:        "STFRIC-1",
		Firmware:     "0.0.1",
	})

	a.Bulb.On.SetValue(isOn)
	if err := a.Bulb.Brightness.SetValue(intialBrightness); err != nil {
		return errors.Wrapf(err, "set initial brightness")
	}

	a.Bulb.On.OnValueRemoteUpdate(powerCallback)
	a.Bulb.Brightness.OnValueRemoteUpdate(brightnessCallback)

	fs := hap.NewFsStore("./homekitdb")
	server, err := hap.NewServer(fs, a.A)
	if err != nil {
		return errors.Wrapf(err, "create hap server")
	}

	go func() {
		if err := server.ListenAndServe(ctx); err != nil {
			slog.Error("failed to start hap server", slog.Any("error", err))
		}
	}()

	return nil
}
