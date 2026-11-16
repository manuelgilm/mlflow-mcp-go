package mlflow

import (
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

func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error) {
	u := c.base + apiPrefix + "/runs/get?" + url.Values{"run_id": {runID}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		if readErr != nil {
			body = nil // the HTTP status is the real failure; report that
		}
		return nil, newAPIError(resp.StatusCode, body)
	}

	var out struct {
		Run Run `json:"run"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Run, nil
}
