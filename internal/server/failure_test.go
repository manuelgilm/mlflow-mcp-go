package server

// Failure-path tests. The point of these is not that an error is returned —
// every layer does that — but that the failure is returned in the shape the
// protocol expects for each surface, and that the session survives it.
//
// Note what a panic would do here: it would take the whole test binary down,
// not fail one case. That bluntness is deliberate (§12) — do not "fix" it with
// a recover() in the handler.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textOf concatenates the text content of a tool result.
func textOf(res *mcp.CallToolResult) string {
	var s string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			s += tc.Text
		}
	}
	return s
}

func TestGetRunSurvivesBackendFailure(t *testing.T) {
	var down atomic.Bool
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			// Deliberately not JSON: this is the /health failure mode that
			// killed the first version of this server (§12).
			w.Write([]byte("Internal Server Error"))
			return
		}
		w.Write([]byte(runFixture))
	})
	ctx := context.Background()
	call := func() (*mcp.CallToolResult, error) {
		return cs.CallTool(ctx, &mcp.CallToolParams{
			Name:      "get_run",
			Arguments: map[string]any{"run_id": "abc"},
		})
	}

	down.Store(true)
	res, err := call()
	if err != nil {
		t.Fatalf("CallTool returned a protocol error; tool failures must be results: %v", err)
	}
	if !res.IsError {
		t.Fatalf("IsError = false for a 500 from MLflow; content = %v", res.Content)
	}
	if msg := textOf(res); !strings.Contains(msg, "500") || !strings.Contains(msg, "Internal Server Error") {
		t.Errorf("error text = %q, want the status and the raw non-JSON body", msg)
	}

	// The assertion that matters: same session, after the failure, still works.
	down.Store(false)
	res, err = call()
	if err != nil {
		t.Fatalf("session did not survive the failure: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true after recovery: %s", textOf(res))
	}
}

func TestReadRunSurvivesBackendFailure(t *testing.T) {
	var down atomic.Bool
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
			return
		}
		w.Write([]byte(runFixture))
	})
	ctx := context.Background()
	read := func() (*mcp.ReadResourceResult, error) {
		return cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "mlflow://runs/abc"})
	}

	down.Store(true)
	_, err := read()
	if err == nil {
		t.Fatal("ReadResource returned no error; resources have no IsError result")
	}
	var wireErr *jsonrpc.Error
	if !errors.As(err, &wireErr) {
		t.Fatalf("want *jsonrpc.Error, got %T: %v", err, err)
	}
	// A backend failure must not be reported as "not found": that is the one
	// mapping a client would act on, and it would be a lie.
	if wireErr.Code == mcp.CodeResourceNotFound {
		t.Errorf("backend failure masqueraded as not-found: %v", err)
	}

	down.Store(false)
	res, err := read()
	if err != nil {
		t.Fatalf("session did not survive the failure: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("got %d contents after recovery, want 1", len(res.Contents))
	}
}

func TestHandlersSurviveDeadTrackingServer(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // now nothing is listening: connect refused, not an HTTP status

	cs := newSessionForURL(t, deadURL)
	ctx := context.Background()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_run",
		Arguments: map[string]any{"run_id": "abc"},
	})
	if err != nil {
		t.Fatalf("a refused connection became a protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatal("IsError = false for a refused connection")
	}
	if msg := textOf(res); !strings.Contains(strings.ToLower(msg), "connection refused") &&
		!strings.Contains(strings.ToLower(msg), "connect") {
		t.Errorf("error text = %q, want the connection error surfaced", msg)
	}

	// The session is still usable: the failure was a result, not a disconnect.
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_experiments"})
	if err != nil {
		t.Fatalf("session did not survive the refused connection: %v", err)
	}
	if !res.IsError {
		t.Fatal("IsError = false for list_experiments against a dead server")
	}

	// Resources surface the same failure as a protocol error instead.
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "mlflow://experiments"}); err == nil {
		t.Fatal("ReadResource returned no error against a dead server")
	}
}
