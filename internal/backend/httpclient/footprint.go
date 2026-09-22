package httpclient

import (
	"context"
	"net/http"

	"github.com/reece01-dock/axos/internal/footprint"
)

// This file is httpclient.Client's other half for axosd's /v1/footprint*
// routes (internal/api's RAM footprint surface — see internal/footprint),
// used by axosctl's "footprint" command.

// FootprintCurrent is the decoded shape of GET /v1/footprint.
type FootprintCurrent struct {
	Current  footprint.Snapshot  `json:"current"`
	Baseline *footprint.Snapshot `json:"baseline,omitempty"`
}

func (c *Client) Footprint(ctx context.Context) (FootprintCurrent, error) {
	var v FootprintCurrent
	err := c.do(ctx, http.MethodGet, "/v1/footprint", nil, &v)
	return v, err
}

func (c *Client) FootprintHistory(ctx context.Context) ([]footprint.Snapshot, error) {
	var v []footprint.Snapshot
	err := c.do(ctx, http.MethodGet, "/v1/footprint/history", nil, &v)
	return v, err
}

func (c *Client) FootprintSnapshot(ctx context.Context, label, releaseID string) (footprint.Snapshot, error) {
	var v footprint.Snapshot
	err := c.do(ctx, http.MethodPost, "/v1/footprint/snapshot", map[string]interface{}{
		"label": label, "release_id": releaseID,
	}, &v)
	return v, err
}
