package hue

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"log/slog"

	openhue "github.com/openhue/openhue-go"
	"github.com/samvdb/loxone-philips-hue/udp"
)

type Adapter struct {
	home   *openhue.Home
	logger *slog.Logger
}

func NewAdapter(ip, appKey string, logger *slog.Logger) (*Adapter, error) {

	h, err := openhue.NewHome(ip, appKey)
	if err != nil {
		return nil, err
	}

	slog.Debug("connect to home bridge", "ip", ip, "apikey", appKey)
	return &Adapter{home: h, logger: logger.With("module", "hue")}, nil
}

func (a *Adapter) Apply(ctx context.Context, cmd udp.Command) error {
	switch cmd.Domain {

	case "grouped_light":
		return a.applyGroupedLight(ctx, cmd)
	default:
		return fmt.Errorf("unsupported domain: %s", cmd.Domain)
	}
}




func (a *Adapter) applyGroupedLight(ctx context.Context, cmd udp.Command) error {
	id := cmd.ID
	switch cmd.Action {
	case "on":
		val := strings.ToLower(cmd.Value)
		on := val == "true" || val == "1"

		a.logger.Info("set light on/off", "id", id, "on", on)
		// Replace with your openhue call:
		light, _ := a.home.GetGroupedLightById(cmd.ID)
		return a.home.UpdateLight(cmd.ID, openhue.LightPut{
			On: light.Toggle(),
		})
	case "dimmable":
		val, _ := strconv.ParseFloat(cmd.Value, 64)
		// n is 0..100
		b := openhue.Brightness(val)
		a.logger.Info("set light brightness", "id", id, "brightness", b)
		return a.home.UpdateGroupedLight(id, openhue.GroupedLightPut{
			Dimming: &openhue.Dimming{
				Brightness: &b,
			},
		})
	default:
		return fmt.Errorf("unsupported light action: %s", cmd.Action)
	}
}
