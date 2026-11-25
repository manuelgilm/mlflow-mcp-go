package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
)

// rawRunFixture is a realistic runs/get body: long param, system tags including
// a bulky mlflow.log-model.history blob, and info fields Summarise drops.
// Built as a var because the 2000-char note and the log-model history cannot
// be constants.
var rawRunFixture = `{"run":{
  "info":{
    "run_id":"b61a7217f83c4961a6e808f9a5e3a9a7",
    "run_uuid":"b61a7217f83c4961a6e808f9a5e3a9a7",
    "run_name":"long-sweep",
    "experiment_id":"1",
    "user_id":"manuel-gil",
    "status":"FINISHED",
    "start_time":1794809938069,
    "end_time":1794810035317,
    "artifact_uri":"mlflow-artifacts:/1/b61a7217f83c4961a6e808f9a5e3a9a7/artifacts",
    "lifecycle_stage":"active"
  },
  "data":{
    "metrics":[
      {"key":"auc","value":0.7496,"timestamp":1794810035317,"step":499},
      {"key":"f1","value":0.5996,"timestamp":1794810035317,"step":499},
      {"key":"loss","value":1.2496,"timestamp":1794810035317,"step":499}
    ],
    "params":[
      {"key":"model","value":"logreg"},
      {"key":"lr","value":"0.02"},
      {"key":"epochs","value":"100"},
      {"key":"note","value":"` + strings.Repeat("x", 2000) + `"}
    ],
    "tags":[
      {"key":"mlflow.user","value":"manuel-gil"},
      {"key":"mlflow.source.name","value":"scripts/seed_mlflow.py"},
      {"key":"mlflow.source.type","value":"LOCAL"},
      {"key":"mlflow.source.git.commit","value":"1c5214de85044196dcd6e8641a7f019d39d209ef"},
      {"key":"mlflow.source.git.branch","value":"master"},
      {"key":"mlflow.source.git.repoURL","value":"https://github.com/manuelgilm/mlflow-mcp-go.git"},
      {"key":"mlflow.runName","value":"long-sweep"},
      {"key":"mlflow.log-model.history","value":"` + strings.Repeat(`model-flavor-record-`, 120) + `"},
      {"key":"team","value":"ml-platform"},
      {"key":"env","value":"dev"}
    ]
  }
}}`

// TestSummariseReducesSize is the token-cost measurement §8 asks for: the
// shaped get_run output must be a small fraction of the raw runs/get body.
// Across the six seeded fixtures the measured ratio is ~0.19 (81% smaller).
// The assertion bars gross bloat, not exact numbers, so a future field added
// to RunSummary that pulls the whole payload back in fails loudly.
func TestSummariseReducesSize(t *testing.T) {
	var envelope struct {
		Run mlflow.Run `json:"run"`
	}
	if err := json.Unmarshal([]byte(rawRunFixture), &envelope); err != nil {
		t.Fatalf("fixture does not decode: %v", err)
	}

	summary, err := json.Marshal(Summarise(&envelope.Run))
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}

	rawBytes, summaryBytes := len(rawRunFixture), len(summary)
	t.Logf("raw=%dB summary=%dB ratio=%.2f", rawBytes, summaryBytes, float64(summaryBytes)/float64(rawBytes))

	if summaryBytes >= rawBytes {
		t.Fatalf("summary (%dB) is not smaller than raw (%dB)", summaryBytes, rawBytes)
	}
	if summaryBytes > rawBytes/2 {
		t.Errorf("summary (%dB) is more than half the raw payload (%dB): get_run has stopped shaping", summaryBytes, rawBytes)
	}
}
