package accessory

import (
	hapaccessory "github.com/brutella/hap/accessory"
	"github.com/cybre/yeelight-controller/internal/homekit/service"
)

type SpotifySyncBulb struct {
	*hapaccessory.A
	Bulb *service.SpotifySyncBulb
}

func NewLightbulb(info hapaccessory.Info) *SpotifySyncBulb {
	a := SpotifySyncBulb{}
	a.A = hapaccessory.New(info, hapaccessory.TypeLightbulb)

	a.Bulb = service.NewSpotifySyncBulb()
	a.AddS(a.Bulb.S)

	return &a
}
