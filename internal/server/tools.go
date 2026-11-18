package server

import (
	"context"
	"errors"
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
		Name: "list_experiments",
		Description: "Answer \"what experiments exist?\" — returns id, name and " +
			"lifecycle stage for every active experiment. Use it to find the " +
			"experiment_id that search_runs filters on.",
	}, h.listExperiments)

	mcp.AddTool(server, &mcp.Tool{
		Name: "search_runs",
		Description: "Answer \"which runs match this filter, or which are best by " +
			"this metric?\" — returns up to max_results runs with status and their " +
			"latest metric values. Use filter for params/metrics/tags expressions " +
			"and order_by to rank them; then get_run for the one you pick.",
	}, h.searchRuns)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_run",
		Description: "Answer \"what happened in this run?\" — returns params, " +
			"latest value per metric, and selected tags.",
	}, h.getRun)
}

// listExperimentsInput is empty on purpose: the tool takes no arguments.
type listExperimentsInput struct{}

func (h *Handlers) listExperiments(ctx context.Context, _ *mcp.CallToolRequest, _ listExperimentsInput) (*mcp.CallToolResult, ExperimentList, error) {
	list, err := h.experiments(ctx)
	if err != nil {
		return nil, ExperimentList{}, err
	}
	return nil, list, nil
}

// experiments is the single source for both list_experiments and
// mlflow://experiments, so the tool and the resource cannot drift.
func (h *Handlers) experiments(ctx context.Context) (ExperimentList, error) {
	// Pagination is milestone 5; 1000 is the largest page every server must
	// support, so this only truncates past a thousand experiments.
	resp, err := h.client.SearchExperiments(ctx, mlflow.SearchExperimentsRequest{MaxResults: 1000})
	if err != nil {
		return ExperimentList{}, err
	}
	if resp.Experiments == nil {
		return ExperimentList{Experiments: []mlflow.Experiment{}}, nil // [] rather than null
	}
	return ExperimentList{Experiments: resp.Experiments}, nil
}

// searchRunsInput is the first tool input with optional fields: every field
// carrying omitempty is one the model may leave out, every field without it is
// one the SDK rejects the call for.
type searchRunsInput struct {
	ExperimentID string   `json:"experiment_id" jsonschema:"experiment to search; list_experiments returns ids"`
	Filter       string   `json:"filter,omitempty" jsonschema:"MLflow filter expression over params, metrics and tags, e.g. metrics.auc > 0.9 and params.model = 'logreg'"`
	OrderBy      []string `json:"order_by,omitempty" jsonschema:"sort order, e.g. [\"metrics.auc DESC\"]; otherwise the server chooses"`
	MaxResults   int      `json:"max_results,omitempty" jsonschema:"how many runs to return (1-200, default 20)"`
}

const (
	defaultSearchResults = 20
	maxSearchResults     = 200
)

func (h *Handlers) searchRuns(ctx context.Context, _ *mcp.CallToolRequest, in searchRunsInput) (*mcp.CallToolResult, SearchRunsOutput, error) {
	if in.ExperimentID == "" {
		return nil, SearchRunsOutput{}, errors.New("experiment_id is required; list_experiments returns the ids")
	}
	limit := in.MaxResults
	if limit == 0 {
		limit = defaultSearchResults
	}
	// A jsonschema tag is description-only, so it cannot express bounds: this is
	// the only place the range is enforced.
	if limit < 1 || limit > maxSearchResults {
		return nil, SearchRunsOutput{}, fmt.Errorf("max_results must be between 1 and %d", maxSearchResults)
	}

	resp, err := h.client.SearchRuns(ctx, mlflow.SearchRunsRequest{
		ExperimentIDs: []string{in.ExperimentID},
		Filter:        in.Filter,
		OrderBy:       in.OrderBy,
		MaxResults:    limit,
	})
	if err != nil {
		return nil, SearchRunsOutput{}, err
	}

	out := SearchRunsOutput{
		Runs:      make([]RunRow, 0, len(resp.Runs)),
		Truncated: resp.HasMore,
	}
	for i := range resp.Runs {
		out.Runs = append(out.Runs, rowOf(&resp.Runs[i]))
	}
	return nil, out, nil
}

func (h *Handlers) getRun(ctx context.Context, _ *mcp.CallToolRequest, in getRunInput) (*mcp.CallToolResult, RunSummary, error) {
	run, err := h.client.GetRun(ctx, in.RunID)
	if err != nil {
		return nil, RunSummary{}, err
	}
	return nil, Summarise(run), nil
}

// SearchRunsOutput is the search result: identity plus metrics, with no params
// or tags. A list of N full RunSummary values would defeat the point of
// searching — get_run exists for the run you decide to read.
type SearchRunsOutput struct {
	Runs      []RunRow `json:"runs"`
	Truncated bool     `json:"truncated,omitempty"`
}

type RunRow struct {
	RunID       string                   `json:"run_id"`
	RunName     string                   `json:"run_name,omitempty"`
	Status      string                   `json:"status"`
	StartedAt   string                   `json:"started_at,omitempty"`
	DurationSec float64                  `json:"duration_seconds,omitempty"`
	Metrics     map[string]MetricSummary `json:"metrics,omitempty"`
}

func rowOf(r *mlflow.Run) RunRow {
	return RunRow{
		RunID:       r.Info.RunID,
		RunName:     r.Info.RunName,
		Status:      r.Info.Status,
		StartedAt:   formatMillis(r.Info.StartTime),
		DurationSec: durationSeconds(r.Info.StartTime, r.Info.EndTime),
		Metrics:     summariseMetrics(r.Data.Metrics),
	}
}

// ExperimentList is the payload shared by the list_experiments tool and the
// mlflow://experiments resource. The envelope leaves room for next_page_token
// when pagination lands, without changing the wire shape.
type ExperimentList struct {
	Experiments []mlflow.Experiment `json:"experiments"`
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
		s.DurationSec = durationSeconds(r.Info.StartTime, r.Info.EndTime)
	}
	return s
}

// durationSeconds is shared by RunSummary and RunRow so the two surfaces agree
// on what a run's duration is.
func durationSeconds(start, end int64) float64 {
	if start <= 0 || end <= start {
		return 0
	}
	return float64(end-start) / 1000
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
