package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
)

// runFixture is a minimal runs/get payload: one metric, one param, one user tag
// and one system tag that Summarise must drop.
const runFixture = `{"run":{
	"info":{"run_id":"abc","run_name":"probe","experiment_id":"1",
	        "status":"FINISHED","start_time":1700000000000,"end_time":1700000004000},
	"data":{
		"metrics":[{"key":"auc","value":0.91,"timestamp":1700000003000,"step":4}],
		"params":[{"key":"lr","value":"0.01"}],
		"tags":[{"key":"mlflow.user","value":"u"},
		        {"key":"mlflow.source.git.commit","value":"deadbeef"},
		        {"key":"team","value":"ml"}]}}}`

// newSession wires a real server to a real client over an in-memory transport,
// backed by a fake MLflow. Tests then assert what a client actually receives on
// the wire, not what the handler returns in memory.
func newSession(t *testing.T, mlflowHandler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()

	mlflowSrv := httptest.NewServer(mlflowHandler)
	t.Cleanup(mlflowSrv.Close)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	New(mlflow.NewClient(mlflowSrv.URL)).RegisterResources(srv)

	cTransport, sTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(context.Background(), sTransport, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), cTransport, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return cs
}

func TestReadRun(t *testing.T) {
	var gotPath, gotRunID string
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRunID = r.URL.Path, r.URL.Query().Get("run_id")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, runFixture)
	})

	const uri = "mlflow://runs/abc"
	res, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}

	if gotPath != "/api/2.0/mlflow/runs/get" || gotRunID != "abc" {
		t.Errorf("upstream call = %s?run_id=%s, want runs/get with run_id=abc", gotPath, gotRunID)
	}
	if res.CacheScope != "private" {
		t.Errorf("CacheScope = %q, want %q (runs are per-server/per-user)", res.CacheScope, "private")
	}
	if len(res.Contents) != 1 {
		t.Fatalf("got %d contents, want 1", len(res.Contents))
	}

	c := res.Contents[0]
	if c.URI != uri {
		t.Errorf("content URI = %q, want %q", c.URI, uri)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("content MIMEType = %q, want application/json", c.MIMEType)
	}

	// Assert the literal payload rather than json.Marshal(Summarise(...)):
	// comparing against Summarise would let a bug in Summarise pass its own test.
	const want = `{"run_id":"abc","run_name":"probe","experiment_id":"1",` +
		`"status":"FINISHED","started_at":"2023-11-14T22:13:20Z","duration_seconds":4,` +
		`"params":{"lr":"0.01"},"metrics":{"auc":{"value":0.91,"step":4}},` +
		`"tags":{"mlflow.user":"u","team":"ml"}}`
	if c.Text != want {
		t.Errorf("content text:\n got %s\nwant %s", c.Text, want)
	}
}

func TestReadRunNotFound(t *testing.T) {
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error_code":"RESOURCE_DOES_NOT_EXIST","message":"Run with id=nope not found"}`)
	})

	_, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "mlflow://runs/nope"})

	var wireErr *jsonrpc.Error
	if !errors.As(err, &wireErr) {
		t.Fatalf("want *jsonrpc.Error, got %T: %v", err, err)
	}
	// An MLflow 404 must surface as "not found", not as an internal error:
	// resources have no IsError equivalent, so an unmapped error reads as a
	// server fault to the client.
	if wireErr.Code != mcp.CodeResourceNotFound {
		t.Errorf("code = %d, want %d (%s)", wireErr.Code, mcp.CodeResourceNotFound, "resource not found")
	}
}

// TestReadRunRawIsNotTheSummary pins the disjointness of the two run templates:
// mlflow://runs/{run_id} must not swallow mlflow://runs/{run_id}/raw, which is
// not registered yet.
func TestReadRunRawIsNotTheSummary(t *testing.T) {
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream called for an unregistered URI: %s", r.URL)
	})

	_, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "mlflow://runs/abc/raw"})

	var wireErr *jsonrpc.Error
	if !errors.As(err, &wireErr) || wireErr.Code != mcp.CodeResourceNotFound {
		t.Fatalf("want resource-not-found, got %v", err)
	}
}

func TestSummarise(t *testing.T) {
	t.Run("empty run omits everything", func(t *testing.T) {
		s := Summarise(&mlflow.Run{Info: mlflow.RunInfo{RunID: "abc", ExperimentID: "1", Status: "FINISHED"}})

		if s.Params != nil || s.Metrics != nil || s.Tags != nil {
			t.Errorf("want nil maps, got params=%v metrics=%v tags=%v", s.Params, s.Metrics, s.Tags)
		}
		if s.StartedAt != "" || s.DurationSec != 0 {
			t.Errorf("want no timing, got started_at=%q duration=%v", s.StartedAt, s.DurationSec)
		}

		body, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		const want = `{"run_id":"abc","experiment_id":"1","status":"FINISHED"}`
		if string(body) != want {
			t.Errorf("got %s, want %s", body, want)
		}
	})

	t.Run("NaN metric serialises as null", func(t *testing.T) {
		s := Summarise(&mlflow.Run{Data: mlflow.RunData{
			Metrics: []mlflow.Metric{{Key: "f1", Value: math.NaN(), Step: 7}},
		}})

		m, ok := s.Metrics["f1"]
		if !ok || m.Value != nil || m.Step != 7 {
			t.Fatalf("got %+v, want nil value with step 7", m)
		}
		// json.Marshal fails outright on a raw NaN; the pointer is what makes
		// this line possible.
		if _, err := json.Marshal(s); err != nil {
			t.Fatalf("marshal of NaN metric: %v", err)
		}
	})

	t.Run("long param is clipped with a marker", func(t *testing.T) {
		long := strings.Repeat("x", 2000)
		s := Summarise(&mlflow.Run{Data: mlflow.RunData{
			Params: []mlflow.Param{{Key: "note", Value: long}},
		}})

		got := s.Params["note"]
		if !strings.HasPrefix(got, strings.Repeat("x", 120)) {
			t.Errorf("value was not clipped to 120 runes: %q", got[:130])
		}
		if !strings.HasSuffix(got, "… (1880 more chars)") {
			t.Errorf("missing truncation marker, got suffix %q", got[len(got)-30:])
		}
	})

	t.Run("system tags are filtered", func(t *testing.T) {
		s := Summarise(&mlflow.Run{Data: mlflow.RunData{Tags: []mlflow.Tag{
			{Key: "mlflow.user", Value: "u"},
			{Key: "mlflow.log-model.history", Value: "{}"},
			{Key: "team", Value: "ml"},
		}}})

		if len(s.Tags) != 2 {
			t.Fatalf("got %v, want mlflow.user + team only", s.Tags)
		}
		if _, ok := s.Tags["mlflow.log-model.history"]; ok {
			t.Error("mlflow.log-model.history was not filtered out")
		}
	})
}
