package client

import (
	"context"
	"log/slog"
	"time"

	openhue "github.com/openhue/openhue-go"
	"github.com/samvdb/loxone-philips-hue/udp"
)

type Poller struct {
	home     *openhue.Home
	receiver *udp.Client
}

func NewPoller(ctx context.Context, hueHome *openhue.Home, udp *udp.Client) *Poller {

	return &Poller{
		home:     hueHome,
		receiver: udp,
	}
}

func (p *Poller) Run() error {
	slog.Debug("poller started at %s", time.Now())
	//
	//devices, err := p.home.GetDevices()
	//if err != nil {
	//	return err
	//}
	//for _, device := range devices {
	//}
	return nil
}
