package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/samvdb/loxone-philips-hue/udp"
	"golang.org/x/net/http2"
)

const backoffMax = 30 * time.Second

func NewStreamer(ctx context.Context, bridgeIP string, hueAPIKey string, udpClient *udp.Client, poller *Poller) EventStreamer {

	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{Transport: &http2.Transport{TLSClientConfig: tlsCfg}}

	return EventStreamer{
		httpClient: client,
		url:        fmt.Sprintf("https://%s/eventstream/clip/v2", bridgeIP),
		apiKey:     hueAPIKey,
		udpClient:  udpClient,
		poller:     poller,
	}

}

func (e *EventStreamer) Run(ctx context.Context) error {
	backoff := time.Second

	for {
		// Exit immediately if we're asked to stop.
		if err := ctx.Err(); err != nil {
			return err
		}

		err := e.streamOnce(ctx)
		if ctx.Err() != nil {
			// Context cancelled while streaming or during request.
			return ctx.Err()
		}
		if err == nil {
			// Clean close from server; reset backoff and continue.
			backoff = time.Second
			continue
		}

		slog.Error(fmt.Sprintf("stream error: %v (reconnecting in %s)", err, backoff))
		if err := sleepContext(ctx, backoff); err != nil {
			return err // ctx cancelled during backoff
		}
		if backoff < backoffMax {
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
	}

}

func (e *EventStreamer) streamOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", e.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("hue-application-key", e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	slog.Info("Listening for Philips Hue Events...")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024) // allow big events

	var buf []byte

	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: blank line separates events; "data:" lines carry payload
		if len(line) == 0 {
			if len(buf) > 0 {
				// parse one complete SSE event payload (JSON array of containers)
				var containers []EventContainer
				if err := json.Unmarshal(buf, &containers); err != nil {
					slog.Error(fmt.Sprintf("bad JSON: %s (err: %v)", string(buf), err))
				} else {
					err := e.handle(ctx, containers)
					if err != nil {
						return err
					}
				}
				buf = buf[:0]
			}
			continue
		}

		if len(line) >= 5 && line[:5] == "data:" {
			// strip "data:" and optional leading space
			payload := line[5:]
			if len(payload) > 0 && payload[0] == ' ' {
				payload = payload[1:]
			}
			// SSE may split data across multiple "data:" lines; join with \n
			if len(buf) > 0 {
				buf = append(buf, '\n')
			}
			buf = append(buf, payload...)
		}
	}

	return scanner.Err()
}

func (e *EventStreamer) handle(ctx context.Context, containers []EventContainer) error {
	for _, c := range containers {
		for _, raw := range c.Data {
			ev, err := decodeResource(raw)
			if err != nil {
				return err
			}

			parent := ev.GetGeneric().Owner

			switch ee := ev.(type) {
			case *LightEvent:
				if ee.On != nil {
					slog.Debug("light event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "on", ee.On.On)
				}
			case *TamperEvent:
				if len(ee.TamperReports) > 0 {
					for _, report := range ee.TamperReports {
						slog.Debug("tamper event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "source", report.Source, "state", report.State)
					}
				}
			case *ContactEvent:
				if ee.ContactReport != nil {
					slog.Debug("contact event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "state", ee.ContactReport.State)
					state := 0
					if ee.ContactReport.State == StateContact {
						state = 1
					}
					e.udpClient.Send([]byte(fmt.Sprintf("/contact/%s/state %b", parent.ID, state)))
				}
			case *MotionEvent:
				if ee.Motion.MotionReport != nil {
					if parent.ID == "" {
						continue
					}
					slog.Debug("motion event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "motion", ee.Motion.MotionReport.Motion)
					value := 0
					// convert to 1 or 0
					if ee.Motion.MotionReport.Motion {
						value = 1
					}
					e.udpClient.Send([]byte(fmt.Sprintf("/sensor/%s/motion %b", parent.ID, value)))
				}

			case *GroupedMotionEvent:
				if ee.Motion.MotionReport != nil {
					if parent.Type == "bridge_home" {
						continue
					}
					slog.Debug("grouped motion event", "id", parent.ID, "group", e.poller.GetDevice(parent.ID), "grouped_motion", ee.Motion.MotionReport.Motion)
					value := 0
					// convert to 1 or 0
					if ee.Motion.MotionReport.Motion {
						value = 1
					}
					e.udpClient.Send([]byte(fmt.Sprintf("/group/%s/motion %b", parent.ID, value)))
				}

			case *LightLevelEvent:
				if ee.Light.LightLevelReport != nil {
					slog.Debug("light level event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "light_level", ee.Light.LightLevelReport.LightLevel)

					e.udpClient.Send([]byte(fmt.Sprintf("/sensor/%s/light_level %f", parent.ID, ee.Light.LightLevelReport.LightLevel)))
				}

			case *GroupedLightLevelEvent:
				if ee.Light.LightLevelReport != nil {
					slog.Debug("grouped light level event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "light_level", ee.Light.LightLevelReport.LightLevel)

					e.udpClient.Send([]byte(fmt.Sprintf("/sensor/%s/light_level %f", parent.ID, ee.Light.LightLevelReport.LightLevel)))
				}

			case *TemperatureEvent:
				if ee.Temperature.TemperatureReport != nil {
					slog.Debug("temperature event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "temperature", ee.Temperature.TemperatureReport.Temperature)

					e.udpClient.Send([]byte(fmt.Sprintf("/sensor/%s/temperature %.2f", parent.ID, ee.Temperature.TemperatureReport.Temperature)))
				}
			case *GroupedLightEvent:
				slog.Debug("grouped_light event", "id", parent.ID, "device", e.poller.GetDevice(parent.ID), "raw", string(raw))
			case *ZigbeeConnectivityEvent:
				slog.Debug("zigbee_connectivity event", "id", parent.ID, "state", ee.Status)

			case *SceneEvent:
				slog.Debug("scene event", "id", parent.ID, "raW", string(raw))
			case *UnknownEvent:
				// keep for diagnostics or forward to a generic handler
				// slog.Debug("unknown event", "type", e.Type, "raw", string(e.Raw))
				slog.Warn("unknown event", "type", ee.Type, "raw", string(ee.Raw))
			case *MutedEvent:

			default:
				slog.Debug("unhandled event", "type", ee.ResourceType())
			}
		}

	}
	return nil
}
