package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/samvdb/loxone-philips-hue/bridge"
)

type Poller struct {
	home    *bridge.Home
	homeIP  string
	homeKey string
	// name index like the Python 'names' map; we try v1 id if available, else fallback.
	mu    sync.RWMutex
	names map[string]Device // key: id_v1 ("/lights/1") OR "<rtype>/<uuid>"

	lastRefresh     time.Time
	refreshInterval time.Duration
}

type Device struct {
	Name  string
	Type  string
	Alias string
	IDv1  string
}

func (d *Device) toString() string {
	return fmt.Sprintf("%s %s - %s ", d.IDv1, d.Name, d.Alias)
}

func NewPoller(ctx context.Context, bridgeIP string, hueAPIKey string) *Poller {

	return &Poller{
		homeIP:          bridgeIP,
		homeKey:         hueAPIKey,
		names:           make(map[string]Device),
		refreshInterval: time.Hour,
	}
}

func (p *Poller) Run(ctx context.Context) error {
	home, err := bridge.NewHome(p.homeIP, p.homeKey)

	if err != nil {
		return err
	}

	p.home = home

	slog.Debug(fmt.Sprintf("poller started at %s", time.Now()))

	if time.Since(p.lastRefresh) >= p.refreshInterval {
		if err := p.refreshNames(ctx); err != nil {
			slog.Warn("refresh names", "err", err)
		} else {
			slog.Info("names refreshed")
		}
		p.lastRefresh = time.Now()
	}

	return nil
}

func (p *Poller) refreshNames(ctx context.Context) error {
	devices, err := p.home.GetDevices()
	if err != nil {
		return err
	}
	for _, device := range devices {
		slog.Info("device", "id", *device.Id, "productName", *device.ProductData.ProductName, "alias", *device.Metadata.Name)
		p.setName(*device.Id, *device.ProductData.ProductName, *device.Metadata.Name, device.IdV1, cleanName(*device.ProductData.ProductName))
	}

	rooms, err := p.home.GetRooms()
	if err != nil {
		return err
	}

	for _, r := range rooms {
		slog.Info("room", "id", *r.Id, "name", *r.Metadata.Name)
		p.setName(*r.Id, "room", *r.Metadata.Name, r.IdV1, "room")
	}

	zones, err := p.home.GetZones(ctx)
	if err != nil {
		return err
	}

	for _, r := range zones {
		slog.Info("zone", "id", *r.Id, "name", *r.Metadata.Name)
	}

	grouped, err := p.home.GetGroupedLights()
	if err != nil {
		return err
	}

	for _, g := range grouped {
		switch *g.Owner.Rtype {
		case "room":
			for _, rr := range rooms {
				if *rr.Id == *g.Owner.Rid {
					slog.Info("grouped_light", "group_id", *g.Id, "room_id", *rr.Id, "room", *rr.Metadata.Name)
					continue
				}
			}
		case "zone":
			for _, rr := range zones {
				if *rr.Id == *g.Owner.Rid {
					slog.Info("grouped_light", "group_id", *g.Id, "zone_id", *rr.Id, "zone", *rr.Metadata.Name)
					continue
				}
			}
			slog.Warn("grouped_light zone", "zone", *g.Id)
		case "bridge_home":
		default:
			return fmt.Errorf("unknown group type: %s", *g.Owner.Rtype)
		}
	}
	return nil
}

func (p *Poller) setName(key, name string, alias string, idv1 *string, t string) {
	if key == "" || name == "" {
		return
	}
	p.mu.Lock()
	idv := ""
	if idv1 != nil {
		idv = *idv1
	}
	p.names[key] = Device{Name: name, Alias: alias, IDv1: idv, Type: t}
	p.mu.Unlock()
}

func (p *Poller) GetDevice(key string) string {
	if key == "" {
		return ""
	}
	if d, ok := p.names[key]; ok {
		return d.toString()
	}
	return ""
}

func (p *Poller) GetName(key string) string {
	if key == "" {
		return ""
	}
	if d, ok := p.names[key]; ok {
		return d.Name
	}
	return ""
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
