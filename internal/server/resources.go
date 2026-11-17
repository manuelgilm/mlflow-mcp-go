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
			"Pass an id to search_runs.",
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
}

func (h *Handlers) readExperiments(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	// Pagination is milestone 5; 1000 is the largest size every server must
	// support, so this only truncates past a thousand experiments.
	resp, err := h.client.SearchExperiments(ctx, mlflow.SearchExperimentsRequest{MaxResults: 1000})
	if err != nil {
		return nil, err
	}

	experiments := resp.Experiments
	if experiments == nil {
		experiments = []mlflow.Experiment{} // marshal as [] rather than null
	}
	body, err := json.Marshal(experiments)
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
	id := strings.TrimPrefix(u.Path, "/")
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
