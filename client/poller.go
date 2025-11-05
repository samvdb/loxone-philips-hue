package client

import (
	"context"

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

}
