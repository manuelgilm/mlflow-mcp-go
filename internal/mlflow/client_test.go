package mlflow

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetRun(t *testing.T) {
	var gotRunID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/2.0/mlflow/runs/get" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotRunID = r.URL.Query().Get("run_id")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"run":{"info":{"run_id":"abc","experiment_id":"0",
			"status":"FINISHED","start_time":1700000000000},
			"data":{"metrics":[{"key":"auc","value":0.91,"timestamp":1,"step":0}],
			"params":[{"key":"lr","value":"0.01"}],"tags":[]}}}`)
	}))
	defer srv.Close()
	run, err := NewClient(srv.URL+"/").GetRun(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRunID != "abc" {
		t.Errorf("run_id query = %q, want %q", gotRunID, "abc")
	}
	if run.Info.ExperimentID != "0" || run.Info.StartTime != 1700000000000 {
		t.Errorf("info = %+v", run.Info)
	}
	if len(run.Data.Metrics) != 1 || run.Data.Metrics[0].Key != "auc" || run.Data.Metrics[0].Value != 0.91 {
		t.Errorf("metrics = %+v", run.Data.Metrics)
	}
	if len(run.Data.Params) != 1 || run.Data.Params[0].Value != "0.01" {
		t.Errorf("params = %+v", run.Data.Params)
	}
}

func TestGetRunNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error_code":"RESOURCE_DOES_NOT_EXIST","message":"no run"}`)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetRun(context.Background(), "missing")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Code != "RESOURCE_DOES_NOT_EXIST" {
		t.Fatalf("got %+v", apiErr)
	}
}

func TestSearchExperiments(t *testing.T) {
	var gotMethod, gotPath, gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		// "workspace" is MLflow 3.x and is deliberately not modelled.
		io.WriteString(w, `{"experiments":[
			{"experiment_id":"0","name":"Default","lifecycle_stage":"active",
			 "workspace":"default"}],"next_page_token":""}`)
	}))
	defer srv.Close()

	resp, err := NewClient(srv.URL).SearchExperiments(context.Background(),
		SearchExperimentsRequest{MaxResults: 1000})
	if err != nil {
		t.Fatalf("SearchExperiments: %v", err)
	}

	// The endpoint is a POST with a JSON body: a GET here would 405 in production
	// while still passing any test that only checked the path.
	if gotMethod != http.MethodPost || gotPath != "/api/2.0/mlflow/experiments/search" {
		t.Errorf("request = %s %s, want POST /api/2.0/mlflow/experiments/search", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody != `{"max_results":1000}` {
		t.Errorf("body = %s", gotBody)
	}
	if len(resp.Experiments) != 1 {
		t.Fatalf("got %d experiments, want 1", len(resp.Experiments))
	}
	if e := resp.Experiments[0]; e.ExperimentID != "0" || e.Name != "Default" || e.LifecycleStage != "active" {
		t.Errorf("experiment = %+v", e)
	}
}
