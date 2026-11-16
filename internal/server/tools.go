package server

import (
	"context"
	"fmt"
	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"math"
	"strings"
	"time"
)

type getRunInput struct {
	RunID string `json:"run_id" jsonschema:"the 32-character run id, as returned by search_runs"`
}

type Handlers struct {
	client *mlflow.Client
}

func New(c *mlflow.Client) *Handlers {
	return &Handlers{client: c}
}

func (h *Handlers) RegisterTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_run",
		Description: "Answer \"what happened in this run?\" — returns params, " +
			"latest value per metric, and selected tags.",
	}, h.getRun)
}

func (h *Handlers) getRun(ctx context.Context, _ *mcp.CallToolRequest, in getRunInput) (*mcp.CallToolResult, RunSummary, error) {
	run, err := h.client.GetRun(ctx, in.RunID)
	if err != nil {
		return nil, RunSummary{}, err
	}
	return nil, Summarise(run), nil
}

type RunSummary struct {
	RunID        string                   `json:"run_id"`
	RunName      string                   `json:"run_name,omitempty"`
	ExperimentID string                   `json:"experiment_id"`
	Status       string                   `json:"status"`
	StartedAt    string                   `json:"started_at,omitempty"` // RFC3339 UTC
	DurationSec  float64                  `json:"duration_seconds,omitempty"`
	Params       map[string]string        `json:"params,omitempty"`
	Metrics      map[string]MetricSummary `json:"metrics,omitempty"`
	Tags         map[string]string        `json:"tags,omitempty"`
}

// Value is a pointer so that a
// logged NaN (undefined recall with zero positives, say) serialises as null
// instead of making json.Marshal fail outright.
type MetricSummary struct {
	Value *float64 `json:"value"`
	Step  int64    `json:"step"`
}

func Summarise(r *mlflow.Run) RunSummary {
	s := RunSummary{
		RunID:        r.Info.RunID,
		RunName:      r.Info.RunName,
		ExperimentID: r.Info.ExperimentID,
		Status:       r.Info.Status,
		StartedAt:    formatMillis(r.Info.StartTime),
		Params:       summariseParams(r.Data.Params),
		Metrics:      summariseMetrics(r.Data.Metrics),
		Tags:         summariseTags(r.Data.Tags),
	}
	if r.Info.EndTime > 0 {
		s.DurationSec = float64(r.Info.EndTime-r.Info.StartTime) / 1000
	}
	return s
}

func summariseParams(ps []mlflow.Param) map[string]string {
	if len(ps) == 0 {
		return nil
	}
	out := make(map[string]string, len(ps))
	for _, p := range ps {
		out[p.Key] = clip(p.Value, 120)
	}
	return out
}

func summariseMetrics(ms []mlflow.Metric) map[string]MetricSummary {
	if len(ms) == 0 {
		return nil
	}
	out := make(map[string]MetricSummary, len(ms))
	for _, m := range ms {
		v := m.Value // runs/get already collapsed history to the latest step
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[m.Key] = MetricSummary{Step: m.Step} // Value stays nil => null
			continue
		}
		out[m.Key] = MetricSummary{Value: &v, Step: m.Step}
	}
	return out
}

var keptTags = map[string]bool{
	"mlflow.user":         true,
	"mlflow.source.type":  true,
	"mlflow.note.content": true,
}

func summariseTags(ts []mlflow.Tag) map[string]string {
	out := make(map[string]string, len(ts))
	for _, t := range ts {
		if strings.HasPrefix(t.Key, "mlflow.") && !keptTags[t.Key] {
			continue
		}
		out[t.Key] = clip(t.Value, 200)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// clip truncates on rune boundaries and says how much was dropped, so the
// model knows a long config blob existed rather than reading a chopped string.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return fmt.Sprintf("%s… (%d more chars)", string(r[:n]), len(r)-n)
}
