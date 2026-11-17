package mlflow

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiPrefix = "/api/2.0/mlflow"
const timeOut = 10

type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: timeOut * time.Second},
	}
}

// do performs one API request against the tracking server and decodes a 2xx
// JSON response into out. A nil body sends no payload; a nil out discards the
// response body. Non-2xx responses become *APIError.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.base + apiPrefix + path
	if encoded := query.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if readErr != nil {
			raw = nil // the HTTP status is the real failure; report that
		}
		return newAPIError(resp.StatusCode, raw)
	}

	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error) {
	var out struct {
		Run Run `json:"run"`
	}
	if err := c.do(ctx, http.MethodGet, "/runs/get", url.Values{"run_id": {runID}}, nil, &out); err != nil {
		return nil, err
	}
	return &out.Run, nil
}

// SearchExperiments lists experiments. The endpoint is a POST with a JSON body,
// not a GET with query params.
func (c *Client) SearchExperiments(ctx context.Context, req SearchExperimentsRequest) (*SearchExperimentsResponse, error) {
	var out SearchExperimentsResponse
	if err := c.do(ctx, http.MethodPost, "/experiments/search", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
