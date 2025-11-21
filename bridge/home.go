package bridge

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"

	openhue "github.com/openhue/openhue-go"
)

type Home struct {
	api *openhue.ClientWithResponses
	*openhue.Home
}

func NewHome(bridgeIP, apiKey string) (*Home, error) {
	if bridgeIP == "" || apiKey == "" {
		return nil, errors.New("illegal arguments, bridgeIP and apiKey must be set")
	}

	base, err := openhue.NewHome(bridgeIP, apiKey)
	if err != nil {
		return nil, err
	}

	client, err := newClient(bridgeIP, apiKey)
	if err != nil {
		return nil, err
	}

	return &Home{
		api:  client,
		Home: base,
	}, nil
}

func (h *Home) GetZones(ctx context.Context) (map[string]openhue.RoomGet, error) {
	resp, err := h.api.GetZonesWithResponse(ctx)
	if err != nil {
		return nil, err
	}

	if resp.HTTPResponse.StatusCode != http.StatusOK {
		return nil, newApiError(resp) // copy or re-implement same logic
	}

	data := *(*resp.JSON200).Data
	zones := make(map[string]openhue.RoomGet, len(data))

	for _, zone := range data {
		zones[*zone.Id] = zone
	}

	return zones, nil
}

// newClient creates a new ClientWithResponses for a given Bridge IP and API key.
// This function will also skip SSL verification, as the Philips HUE Bridge exposes a self-signed certificate.
func newClient(bridgeIP, apiKey string) (*openhue.ClientWithResponses, error) {

	var authFn openhue.RequestEditorFn

	if len(apiKey) > 0 {
		authFn = func(ctx context.Context, req *http.Request) error {
			req.Header.Set("hue-application-key", apiKey)
			return nil
		}
	} else {
		authFn = func(ctx context.Context, req *http.Request) error {
			return nil
		}
	}

	// skip SSL Verification
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return openhue.NewClientWithResponses("https://"+bridgeIP, openhue.WithRequestEditorFn(authFn))
}
