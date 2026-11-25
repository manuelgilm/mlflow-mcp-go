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

// GetRunRaw returns the full, unshaped runs/get response for a run, exactly as
// MLflow returned it (JSON-normalised). No metric collapse, no tag filtering,
// no field omission — this is the backing for mlflow://runs/{id}/raw.
func (c *Client) GetRunRaw(ctx context.Context, runID string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/runs/get", url.Values{"run_id": {runID}}, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
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

// maxPageSize is the largest page every MLflow server must accept.
const maxPageSize = 1000

// searchRunsBody is the wire shape of runs/search. PageToken is client-owned:
// callers ask for a result count, not for a page.
type searchRunsBody struct {
	ExperimentIDs []string `json:"experiment_ids"`
	Filter        string   `json:"filter,omitempty"`
	OrderBy       []string `json:"order_by,omitempty"`
	MaxResults    int      `json:"max_results,omitempty"`
	PageToken     string   `json:"page_token,omitempty"`
}

// SearchRuns returns up to req.MaxResults runs, following page tokens until it
// has that many or the server runs out. HasMore reports that more runs matched
// than were returned, so the caller can say so rather than silently truncating.
func (c *Client) SearchRuns(ctx context.Context, req SearchRunsRequest) (*SearchRunsResponse, error) {
	want := req.MaxResults
	if want <= 0 {
		want = maxPageSize
	}

	var runs []Run
	pageToken := ""
	hasMore := false

	for {
		pageSize := want - len(runs)
		if pageSize > maxPageSize {
			pageSize = maxPageSize
		}

		var page struct {
			Runs          []Run  `json:"runs"`
			NextPageToken string `json:"next_page_token"`
		}
		body := searchRunsBody{
			ExperimentIDs: req.ExperimentIDs,
			Filter:        req.Filter,
			OrderBy:       req.OrderBy,
			MaxResults:    pageSize,
			PageToken:     pageToken,
		}
		if err := c.do(ctx, http.MethodPost, "/runs/search", nil, body, &page); err != nil {
			return nil, err
		}
		runs = append(runs, page.Runs...)

		pageToken = page.NextPageToken
		if pageToken == "" {
			break // exhausted
		}
		if len(runs) == 0 {
			// A token with no rows would loop forever; treat it as the end.
			hasMore = true
			break
		}
		if len(runs) >= want {
			hasMore = true
			break
		}
	}

	if len(runs) > want {
		runs = runs[:want] // servers may return more than asked
	}
	return &SearchRunsResponse{Runs: runs, HasMore: hasMore}, nil
}
