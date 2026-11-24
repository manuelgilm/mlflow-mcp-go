package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCompareRunsPrompt lists the prompt and checks the rendered message, on
// the wire. The fake MLflow is never called: prompts must instruct the model,
// not fetch data themselves.
func TestCompareRunsPrompt(t *testing.T) {
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("prompt handler called MLflow: %s", r.URL)
	})

	listed, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(listed.Prompts) != 1 || listed.Prompts[0].Name != "compare-runs" {
		t.Fatalf("prompts = %+v", listed.Prompts)
	}
	args := listed.Prompts[0].Arguments
	if len(args) != 2 || !args[0].Required {
		t.Errorf("arguments = %+v, want run_ids required plus optional metric", args)
	}

	res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "compare-runs",
		Arguments: map[string]string{"run_ids": "a, b", "metric": "auc"},
	})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(res.Messages))
	}

	msg, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want text", res.Messages[0].Content)
	}
	if res.Messages[0].Role != "user" {
		t.Errorf("role = %q, want user", res.Messages[0].Role)
	}
	for _, want := range []string{"- a\n", "- b\n", "auc", "report only what differs", "search_runs first"} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("message missing %q:\n%s", want, msg.Text)
		}
	}
}

func TestCompareRunsPromptRejectsFewerThanTwo(t *testing.T) {
	cs := newSession(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("prompt handler called MLflow: %s", r.URL)
	})

	for _, runIDs := range []string{"", "only-one", "   "} {
		_, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{
			Name:      "compare-runs",
			Arguments: map[string]string{"run_ids": runIDs},
		})
		var wireErr *jsonrpc.Error
		if !errors.As(err, &wireErr) {
			t.Fatalf("run_ids=%q: want *jsonrpc.Error, got %T: %v", runIDs, err, err)
		}
		// Invalid arguments, not "the server is broken": a plain error would
		// surface as CodeInternalError and mislead every client.
		if wireErr.Code != jsonrpc.CodeInvalidParams {
			t.Errorf("run_ids=%q: code = %d, want %d", runIDs, wireErr.Code, jsonrpc.CodeInvalidParams)
		}
	}
}
