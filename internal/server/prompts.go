package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterPrompts wires the compare-runs prompt. Unlike tools, prompts have no
// schema inference and their Required argument flag is advisory only: the SDK
// looks the name up and hands the whole request to this handler.
func (h *Handlers) RegisterPrompts(srv *mcp.Server) {
	srv.AddPrompt(&mcp.Prompt{
		Name:  "compare-runs",
		Title: "Compare runs",
		Description: "Compare two or more runs of the same experiment: which " +
			"params differ, and how the metrics moved.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "run_ids",
				Description: "comma-separated run ids, at least two (search_runs returns them)",
				Required:    true,
			},
			{
				Name:        "metric",
				Description: "the metric that decides which run is better, e.g. auc",
			},
		},
	}, h.compareRuns)
}

func (h *Handlers) compareRuns(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	var ids []string
	for _, id := range strings.Split(req.Params.Arguments["run_ids"], ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 {
		// Prompts have no IsError equivalent. A plain error would surface as
		// CodeInternalError ("the server is broken"), which is a lie: the
		// arguments were wrong. This is the same code the SDK uses for an
		// unknown prompt name.
		return nil, &jsonrpc.Error{
			Code:    jsonrpc.CodeInvalidParams,
			Message: "run_ids must name at least two runs",
		}
	}

	metric := strings.TrimSpace(req.Params.Arguments["metric"])

	var b strings.Builder
	b.WriteString("Fetch each of these runs with the get_run tool:\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s\n", id)
	}
	b.WriteString("\nThen compare them and report only what differs:\n")
	b.WriteString("1. Params present in some runs but not others, and params whose values differ (old → new).\n")
	b.WriteString("2. Metrics as deltas, with the step each value came from. Skip any metric that is identical.\n")
	b.WriteString("3. One sentence naming the run you would keep and why")
	if metric != "" {
		fmt.Fprintf(&b, ", judged on %s", metric)
	}
	b.WriteString(".\n\nDo not restate params or metrics that are the same across all runs. " +
		"Do not paste raw run JSON. If you do not have run ids yet, call search_runs first.\n")

	return &mcp.GetPromptResult{
		Description: "Compare " + strings.Join(ids, ", "),
		Messages: []*mcp.PromptMessage{{
			Role:    "user",
			Content: &mcp.TextContent{Text: b.String()},
		}},
	}, nil
}
