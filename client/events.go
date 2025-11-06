package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/samvdb/loxone-philips-hue/udp"
)

type EventContainer struct {
	// The Hue bridge sends an array of "events", each with a "type" and "data".
	// We keep this generic; shape varies by resource.
	CreationTime time.Time         `json:"creationtime"`
	ID           string            `json:"id"`
	Type         EventType         `json:"type"`
	Owner        interface{}       `json:"owner"`
	Data         []json.RawMessage `json:"data"`
}

type EventResource interface {
	ResourceType() string
}

type EventType string

type OnEvent struct {
	On bool `json:"on"`
}

type EventStreamer struct {
	httpClient *http.Client
	url        string
	apiKey     string
	udpClient  *udp.Client
}

const (
	EventTypeUpdate EventType = "update"
)

type Owner struct {
	ID   string `json:"rid`
	Type string `json:"rtype`
}

type GenericEvent struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Owner Owner  `json:"owner"`
}

type LightEvent struct {
	*GenericEvent
	// Hue v2 typically nests this like: "on": {"on": true}
	On *struct {
		On bool `json:"on"`
	} `json:"on,omitempty"`
}

func (e *LightEvent) ResourceType() string { return e.Type }

type ContactEvent struct {
	*GenericEvent
	ContactReport *struct {
		State   ContactState `json:"state"`             // e.g. "open"/"closed"
		Changed *time.Time   `json:"changed,omitempty"` // if present
	} `json:"contact_report,omitempty"`
}

func (e *ContactEvent) ResourceType() string { return e.Type }

type TamperEvent struct {
	*GenericEvent
	TamperReports []*struct {
		Source  string      `json:"source`
		State   TamperState `json:"state"`
		Changed *time.Time  `json:"changed,omitempty"`
	} `json:"tamper_reports,omitempty"`
}

func (e *TamperEvent) ResourceType() string { return e.Type }

type ZigbeeConnectivityEvent struct {
	*GenericEvent
	IDv1   string          `json:"id_v1`
	Status ConnectedStatus `json:"status`
}

func (e *ZigbeeConnectivityEvent) ResourceType() string { return e.Type }

type SceneEvent struct {
	*GenericEvent
	IDv1   string `json:"id_v1`
	Status struct {
		Active     string    `json:"active"`
		LastRecall time.Time `json:"last_recall"`
	} `json:"status`
}

func (e *SceneEvent) ResourceType() string { return e.Type }

type GroupedLightEvent struct {
	*GenericEvent
	IDv1    string `json:"id_v1`
	Dimming struct {
		Brightness float64 `json:"brightness"`
	} `json:"dimming`
}

func (e *GroupedLightEvent) ResourceType() string { return e.Type }

type ContactState string

const (
	StateContact   ContactState = "contact"
	StateNoContact ContactState = "no_contact"
)

type ConnectedStatus string

const (
	StatusConnected    ConnectedStatus = "connected"
	StatusDisconnected ConnectedStatus = "connectivity_issue"
)

type TamperState string

const (
	StateTampered    TamperState = "tampered"
	StateNotTampered TamperState = "not_tampered"
)

// Minimal probe to read only the "type" field.
type typeProbe struct {
	Type string `json:"type"`
}

// Decode one raw data object into a concrete EventResource.
func decodeResource(b []byte) (EventResource, error) {
	var tp typeProbe
	if err := json.Unmarshal(b, &tp); err != nil {
		return nil, fmt.Errorf("peek type: %w", err)
	}
	switch tp.Type {
	case "light":
		var ev LightEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("light: %w", err)
		}
		return &ev, nil
	case "contact":
		var ev ContactEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("contact: %w", err)
		}
		return &ev, nil
	case "tamper":
		var ev TamperEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("tamper: %w", err)
		}
		return &ev, nil

	case "zigbee_connectivity":
		var ev ZigbeeConnectivityEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("zigbee_connectivity: %w", err)
		}
		return &ev, nil
	case "scene":
		var ev SceneEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("scene: %w", err)
		}
		return &ev, nil

	case "grouped_light":
		var ev GroupedLightEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("grouped_light: %w", err)
		}
		return &ev, nil
	// add other resource types here: "motion", "button", "temperature", ...
	default:
		// Unknown type? Return a raw wrapper so you don’t lose data.
		return &UnknownEvent{Raw: b, Type: tp.Type}, nil
	}
}

type UnknownEvent struct {
	Type string
	Raw  []byte
}

func (e *UnknownEvent) ResourceType() string { return e.Type }
