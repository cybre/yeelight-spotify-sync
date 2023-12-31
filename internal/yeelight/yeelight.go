package yeelight

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/cybre/yeelight-controller/internal/errors"
)

const (
	// yeelight discover message for SSDP
	discoverMSG = "M-SEARCH * HTTP/1.1\r\n HOST:239.255.255.250:1982\r\n MAN:\"ssdp:discover\"\r\n ST:wifi_bulb\r\n"
	// timeout value for TCP and UDP commands
	timeout = time.Second * 3
	// SSDP discover address
	ssdpAddress = "239.255.255.250:1982"
	// line ending (CRLF)
	lineEnding = "\r\n"
)

func Discover(ctx context.Context) ([]Bulb, error) {
	bulbs := make([]Bulb, 0)

	udpAddr, err := net.ResolveUDPAddr("udp4", ssdpAddress)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve SSDP address")
	}

	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return nil, errors.Wrapf(err, "establish connection to SSDP address")
	}

	if _, err = conn.WriteToUDP([]byte(discoverMSG), udpAddr); err != nil {
		return nil, errors.Wrapf(err, "write discover message to SSDP address")
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, errors.Wrapf(err, "set read deadline for SSDP connection")
	}

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, errors.Wrapf(err, "read from SSDP connection")
	}

	resp := string(buf[:n])
	lines := strings.Split(resp, lineEnding)

	for _, line := range lines {
		if strings.HasPrefix(line, "Location: yeelight://") {
			address := strings.TrimPrefix(line, "Location: yeelight://")
			addr, err := netip.ParseAddrPort(address)
			if err != nil {
				return nil, errors.Wrapf(err, "parse bulb address")
			}
			bulbs = append(bulbs, newBulb(addr))
		}

		if strings.HasPrefix(line, "id:") {
			id := strings.TrimPrefix(line, "id: ")
			bulbs[len(bulbs)-1].id = id
		}

		if strings.HasPrefix(line, "support:") {
			support := strings.TrimPrefix(line, "support: ")
			supportArr := strings.Split(support, " ")
			bulbs[len(bulbs)-1].support = supportArr
		}

		if strings.HasPrefix(line, "power:") {
			power := strings.TrimPrefix(line, "power: ")
			bulbs[len(bulbs)-1].power = PowerStatus(power)
		}

		if strings.HasPrefix(line, "bright:") {
			brightness := strings.TrimPrefix(line, "bright: ")
			brightnessInt, err := strconv.ParseUint(brightness, 10, 8)
			if err != nil {
				return nil, errors.Wrapf(err, "convert brightness to int")
			}
			bulbs[len(bulbs)-1].brightness = uint8(brightnessInt)
		}

		if strings.HasPrefix(line, "color_mode:") {
			colorMode := strings.TrimPrefix(line, "color_mode: ")
			colorModeInt, err := strconv.ParseUint(colorMode, 10, 8)
			if err != nil {
				return nil, errors.Wrapf(err, "convert color mode to int")
			}
			bulbs[len(bulbs)-1].colorMode = ColorMode(colorModeInt)
		}

		if strings.HasPrefix(line, "ct:") {
			colorTemperature := strings.TrimPrefix(line, "ct: ")
			colorTemperatureInt, err := strconv.ParseUint(colorTemperature, 10, 16)
			if err != nil {
				return nil, errors.Wrapf(err, " convert color temperature to int")
			}
			bulbs[len(bulbs)-1].colorTemperature = uint16(colorTemperatureInt)
		}

		if strings.HasPrefix(line, "rgb:") {
			rgb := strings.TrimPrefix(line, "rgb: ")
			rgbInt, err := strconv.ParseUint(rgb, 10, 32)
			if err != nil {
				return nil, errors.Wrapf(err, "convert RGB to int")
			}
			bulbs[len(bulbs)-1].rgb = uint(rgbInt)
		}

		if strings.HasPrefix(line, "hue:") {
			hue := strings.TrimPrefix(line, "hue: ")
			hueInt, err := strconv.ParseUint(hue, 10, 16)
			if err != nil {
				return nil, errors.Wrapf(err, "convert hue to int")
			}
			bulbs[len(bulbs)-1].hue = uint16(hueInt)
		}

		if strings.HasPrefix(line, "sat:") {
			saturation := strings.TrimPrefix(line, "sat: ")
			saturationInt, err := strconv.ParseUint(saturation, 10, 8)
			if err != nil {
				return nil, errors.Wrapf(err, "convert saturation to int")
			}
			bulbs[len(bulbs)-1].saturation = uint8(saturationInt)
		}

		if strings.HasPrefix(line, "name:") {
			name := strings.TrimPrefix(line, "name: ")
			bulbs[len(bulbs)-1].name = name
		}

		if strings.HasPrefix(line, "model:") {
			model := strings.TrimPrefix(line, "model: ")
			bulbs[len(bulbs)-1].model = model
		}

		if strings.HasPrefix(line, "fw_ver:") {
			firmwareVersion := strings.TrimPrefix(line, "fw_ver: ")
			bulbs[len(bulbs)-1].firmwareVersion = firmwareVersion
		}
	}

	return bulbs, nil
}
