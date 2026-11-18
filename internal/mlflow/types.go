package mlflow

type Run struct {
	Info RunInfo `json:"info"`
	Data RunData `json:"data"`
}

type RunInfo struct {
	RunID          string `json:"run_id"`
	RunName        string `json:"run_name"`
	ExperimentID   string `json:"experiment_id"`
	Status         string `json:"status"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	ArtifactURI    string `json:"artifact_uri"`
	LifecycleStage string `json:"lifecycle_stage"`
}

type RunData struct {
	Metrics []Metric `json:"metrics"`
	Params  []Param  `json:"params"`
	Tags    []Tag    `json:"tags"`
}

type Metric struct {
	Key       string  `json:"key"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
	Step      int64   `json:"step"`
}

type Param struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Experiment struct {
	ExperimentID   string `json:"experiment_id"`
	Name           string `json:"name"`
	LifecycleStage string `json:"lifecycle_stage"`
}

type SearchExperimentsRequest struct {
	MaxResults int    `json:"max_results,omitempty"`
	PageToken  string `json:"page_token,omitempty"`
}

type SearchExperimentsResponse struct {
	Experiments   []Experiment `json:"experiments"`
	NextPageToken string       `json:"next_page_token"`
}

type SearchRunsRequest struct {
	ExperimentIDs []string
	Filter        string
	OrderBy       []string
	MaxResults    int
}

// SearchRunsResponse holds the merged result of following page tokens.
// The client owns pagination, so callers see HasMore rather than a token.
type SearchRunsResponse struct {
	Runs    []Run
	HasMore bool
}
