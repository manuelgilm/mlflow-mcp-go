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
