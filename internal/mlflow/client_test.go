package mlflow

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"context"
	"errors" 
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
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Code != "RESOURCE_DOES_NOT_EXIST" {
		t.Fatalf("got %+v", apiErr)
	}
}
