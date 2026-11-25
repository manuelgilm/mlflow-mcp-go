package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"strings"
)

func (h *Handlers) RegisterResources(srv *mcp.Server) {
	srv.AddResource(&mcp.Resource{
		URI:   "mlflow://experiments",
		Name:  "experiments",
		Title: "Experiments",
		Description: "Every active experiment: id, name and lifecycle stage. " +
			"Same data as the list_experiments tool.",
		MIMEType: "application/json",
	}, h.readExperiments)

	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "mlflow://runs/{run_id}",
		Name:        "run",
		Title:       "Run Summary",
		Description: "Summary of one run: Params, latest value per metric " +
			"(with its step), and user tags. Same Shape as the get_run tool.",
		MIMEType: "application/json",
	}, h.readRun)

	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "mlflow://runs/{run_id}/raw",
		Name:        "run-raw",
		Title:       "Run (raw JSON)",
		Description: "The full, unshaped runs/get response for one run — no " +
			"metric collapse, no tag filtering, no field omission. Opt-in: it can be large.",
		MIMEType: "application/json",
	}, h.readRunRaw)
}

// runID extracts the run id from the path of a matched template. Both run
// templates route here so their parsing cannot drift.
func runID(path string) string {
	return strings.TrimPrefix(strings.TrimSuffix(path, "/raw"), "/")
}

func (h *Handlers) readRunRaw(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI) // Host="runs", Path="/<id>/raw"
	if err != nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	id := runID(u.Path)
	if id == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	raw, err := h.client.GetRunRaw(ctx, id)
	if err != nil {
		var apiErr *mlflow.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Cacheable: mcp.Cacheable{CacheScope: "private"},
		Contents:  []*mcp.ResourceContents{{Text: string(raw)}},
	}, nil
}

func (h *Handlers) readExperiments(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	list, err := h.experiments(ctx)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Cacheable: mcp.Cacheable{CacheScope: "private"},
		Contents:  []*mcp.ResourceContents{{Text: string(body)}},
	}, nil
}

func (h *Handlers) readRun(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI) // Host="runs", Path="/<id>"
	if err != nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	id := runID(u.Path)
	if id == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	run, err := h.client.GetRun(ctx, id)
	if err != nil {
		var apiErr *mlflow.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return nil, err
	}

	body, err := json.Marshal(Summarise(run))
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Cacheable: mcp.Cacheable{CacheScope: "private"},          // runs are per-server/per-user
		Contents:  []*mcp.ResourceContents{{Text: string(body)}}, // URI/MIMEType filled by SDK
	}, nil
}
