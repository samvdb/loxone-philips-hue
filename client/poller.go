package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	openhue "github.com/openhue/openhue-go"
	"github.com/samvdb/loxone-philips-hue/udp"
)

type Poller struct {
	home     *openhue.Home
	receiver *udp.Client
	// name index like the Python 'names' map; we try v1 id if available, else fallback.
	mu              sync.RWMutex
	names           map[string]string // key: id_v1 ("/lights/1") OR "<rtype>/<uuid>"
	lastRefresh     time.Time
	refreshInterval time.Duration
}

func NewPoller(hueHome *openhue.Home, udp *udp.Client) *Poller {

	return &Poller{
		home:            hueHome,
		receiver:        udp,
		names:           make(map[string]string),
		refreshInterval: time.Hour,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	slog.Debug(fmt.Sprintf("poller started at %s", time.Now()))

	if time.Since(p.lastRefresh) >= p.refreshInterval {
		if err := p.refreshNames(ctx); err != nil {
			slog.Warn("refresh names", "err", err)
		} else {
			slog.Info("names refreshed")
		}
		p.lastRefresh = time.Now()
	}

	devices, err := p.home.GetDevices()
	if err != nil {
		return err
	}
	for _, device := range devices {
		slog.Info("device", "id", *device.Id, "productName", *device.ProductData.ProductName, "alias", *device.Metadata.Name)
	}

	rooms, _ := p.home.GetRooms()
	for _, room := range rooms {
		slog.Info("room", "id", *room.Id, "name", *room.Metadata.Name)
	}

	return nil
}

func (p *Poller) refreshNames(ctx context.Context) error {
	lights, err := p.home.GetLights()
	if err != nil {
		return err
	}
	for _, l := range lights {
		key := firstNonEmpty(*l.IdV1, fmt.Sprintf("/light/%s", *l.Id))
		p.setName(key, *l.Metadata.Name)
	}

	// Groups (Hue v2: "room"/"zone" → grouped_light; we also add special /groups/0)
	rs, err := p.home.GetRooms()
	if err != nil {
		return err
	}
	for _, r := range rs {
		key := firstNonEmpty(*r.IdV1, fmt.Sprintf("/grouped_light/%s", *r.Id))
		p.setName(key, *r.Metadata.Name)
	}
	p.setName("/groups/0", "All lights")

	// Groups (Hue v2: "room"/"zone" → grouped_light; we also add special /groups/0)
	res, err := p.home.GetResources()
	if err != nil {
		return err
	}
	for _, r := range res {

		if *r.Id != "e9b43fe4-cbf7-47f5-916b-b24e9d0f4652" {
			continue
		}

		device, err := p.home.GetDeviceById(*r.Id)
		if err != nil {
			return err
		}

		key := firstNonEmpty(*device.IdV1, fmt.Sprintf("/scenes/%s", *r.Id))
		p.setName(key, *device.Metadata.Name)
	}
	p.setName("/scenes/0", "All Scenes")

	return nil
}

func (p *Poller) setName(key, name string) {
	if key == "" || name == "" {
		return
	}
	p.mu.Lock()
	p.names[key] = name
	p.mu.Unlock()
}

// func (p *Poller) nameFor(r openhue.Resource, fallback string) string {
// 	// prefer v1 path
// 	if *r.IdV1 != "" {
// 		p.mu.RLock()
// 		if n, ok := p.names[*r.IdV1]; ok {
// 			p.mu.RUnlock()
// 			return n
// 		}
// 		p.mu.RUnlock()
// 	}
// 	// try "<rtype>/<uuid>"
// 	key := fmt.Sprintf("/%s/%s", strings.ToLower(*r.Type), *r.Id)
// 	p.mu.RLock()
// 	n := p.names[key]
// 	p.mu.RUnlock()
// 	if n != "" {
// 		return n
// 	}
// 	return fallback
// }
