package service

import (
	"github.com/brutella/hap/characteristic"
	hapservice "github.com/brutella/hap/service"
)

const TypeLightbulb = "43"

type SpotifySyncBulb struct {
	*hapservice.S

	On         *characteristic.On
	Brightness *characteristic.Brightness
}

func NewSpotifySyncBulb() *SpotifySyncBulb {
	s := SpotifySyncBulb{}
	s.S = hapservice.New(TypeLightbulb)

	s.On = characteristic.NewOn()
	s.AddC(s.On.C)

	s.Brightness = characteristic.NewBrightness()
	s.AddC(s.Brightness.C)

	return &s
}
