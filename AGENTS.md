# AGENTS.md — mlflow-mcp-go

Context for any agent working in this repository. Read this before proposing changes.

---

## 1. What this is

An MCP server, written in Go with the **official** SDK (`github.com/modelcontextprotocol/go-sdk`), that exposes an MLflow tracking server to coding agents: search experiments and runs, read a run, compare runs.

It is a **read-first server**. Mutations (logging metrics, creating experiments, registry promotion) are explicitly out of scope for v0.1.

## 2. Why it exists

Three purposes, in priority order:

1. **Learning vehicle.** The author is learning the MCP protocol and Go by writing this by hand. Understanding beats speed.
2. **Content artifact.** It is the subject of a YouTube series about MCP *depth* — tool budgets, error semantics, resources vs tools, failure modes. Anything in the repo may appear on camera, so prefer clear code over clever code.
3. **Reference implementation.** It doubles as the practice deployment for remote MCP (Streamable HTTP, OAuth, per-user secrets).

## 3. Working agreement (important)

- **The human writes the core implementation.** The client, the resources, and the tools are being written by hand, deliberately. Do not take over.
- **Your default role:** explain, review, sketch, propose a diff, or write the boring parts (tests, fixtures, docs) when asked.
- **Never rewrite a file wholesale** unless explicitly asked. Small, reviewable changes.
- **One step per turn.** Do not chain three refactors "while you're in there".
- **Do not guess SDK APIs.** The module cache is the source of truth:
  `/home/manuel-gil/go/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/`
  Look up `server.go` (`AddResource`, `AddResourceTemplate`), `resource.go` (`ResourceHandler`), `protocol.go` (`Resource`, `ResourceTemplate`, `ReadResourceParams`). Verified facts are in §7.4.
- **Do not "improve" the scope.** If something looks missing, ask; it is probably deliberate.

## 4. Scope

| In scope (v0.1) | Out of scope |
| --- | --- |
| `list_experiments`, `search_runs`, `get_run` | Logging metrics/params/tags from the agent |
| Resources: experiments list, run by URI | Creating/deleting experiments or runs |
| `compare-runs` prompt | Model registry, model versions |
| Shaped/summarised output | Artifact download/upload |
| Typed errors, no panics | Auth, retries, caching layers (add when a caller needs them) |
| `httptest` unit tests | Full MLflow SDK surface (~40 endpoints) |

**Rule of thumb:** a tool earns its place only if it answers a question an agent would otherwise answer with several API calls and a lot of reasoning.

## 5. Design rules

1. **Never `panic` in a handler.** Return an error. A panic in a stdio server kills the process and the tool silently disappears from the client's tool list. (This already happened once; see §12.)
2. **Dependency direction:** `internal/mlflow` must not import `mcp`. The client is a plain HTTP client; the server layer adapts it.
3. **No abstraction before the second caller.** No interfaces, no generics, no option structs until something actually needs them.
4. **Return shaped data, not payloads.** A run can carry thousands of metric points. Summarise; offer raw only via an explicit resource.
5. **Structured errors, not strings.** Callers need to distinguish "not found" from "server down" from "bad request".
6. **Resources are for data identified by URI; tools are for actions.** Resources are chosen by the client/user, not the model — so never put the only path to needed data behind a resource.
7. **No new tool without a stated question it answers**, written in the tool description.

## 6. Architecture

```
cmd/mlflow-mcp/main.go     wiring only: config, server, register*()
internal/mlflow/           client.go, types.go, errors.go, client_test.go
internal/server/           tools.go, resources.go, prompts.go
testdata/                  golden JSON fixtures for tests
```

```go
// internal/mlflow
type Client struct { base string; http *http.Client }
func NewClient(baseURL string) *Client
func (c *Client) GetRun(ctx context.Context, runID string) (*Run, error)
func (c *Client) SearchRuns(ctx context.Context, req SearchRunsRequest) (*SearchRunsResponse, error)
func (c *Client) SearchExperiments(ctx context.Context, req SearchExperimentsRequest) (*SearchExperimentsResponse, error)

type APIError struct { Status int; Code string; Message string }
```

No auth, no retry, no cache in `Client` until a caller needs it.

## 7. Interface contract

### 7.1 Resources

| URI | Kind | Contents |
| --- | --- | --- |
| `mlflow://experiments` | concrete `Resource` | Experiment list: id, name, lifecycle stage |
| `mlflow://runs/{run_id}` | `ResourceTemplate` | Run summary: info, params, latest value per metric, selected tags |
| `mlflow://runs/{run_id}/raw` | `ResourceTemplate` | Full MLflow JSON (opt-in, large) |

Do **not** enumerate runs as concrete resources. `resources/list` is called on connect; it must stay small and cheap.

### 7.2 Tools

| Tool | Question it answers |
| --- | --- |
| `list_experiments` | "what experiments exist?" |
| `search_runs` | "which runs match this filter / are best by this metric?" |
| `get_run` | "what happened in this run?" (same data as the resource, model-invocable) |
| `compare_runs` (phase 2) | "what differs between these runs?" — shaped diff, not three full dumps |

### 7.3 Prompt

`compare-runs(experiment_id, run_ids, metric?)` — instructs the model to fetch via `get_run`, diff params/metrics, and report deltas rather than restating everything.

### 7.4 Verified SDK facts (v1.8.0) — do not re-derive

- `func (s *Server) AddResource(r *Resource, h ResourceHandler)`
- `func (s *Server) AddResourceTemplate(t *ResourceTemplate, h ResourceHandler)`
- `type ResourceHandler func(context.Context, *ReadResourceRequest) (*ReadResourceResult, error)`
- `req.Params.URI` is the full URI; parse it yourself.
- In `mlflow://runs/abc`, `url.Parse` yields `Scheme="mlflow"`, `Host="runs"`, `Path="/abc"`.
- `AddResource` / `AddResourceTemplate` **panic** on an invalid URI or template; they return nothing.
- Template resolution iterates a map → **overlapping templates are non-deterministic**. Keep them disjoint.
- The SDK fills `Contents[].URI` and `MIMEType` from the matched resource/template when empty.
- `ReadResourceResult` embeds `Cacheable`; **`CacheScope` defaults to `"public"`** — set `"private"` for anything user-scoped.
- `mcp.ResourceNotFoundError(uri)` for clean not-found errors.
- There is no package-level generic `mcp.AddResource` (unlike `mcp.AddTool`).

### 7.5 MLflow API facts to verify, not assume

- `runs/get` is GET with `?run_id=`; `runs/search` and `experiments/search` are GET with query params.
- Timestamps (`start_time`, `end_time`, `timestamp`) are **milliseconds since epoch**.
- `experiment_id` is a string, even though it looks numeric.
- `max_results` defaults to 1000; pagination uses `page_token`.
- Errors are `{"error_code": "...", "message": "..."}` — but not from every endpoint.
- `run_name` may be empty.

## 8. Milestones

| # | Deliverable | Done when |
| --- | --- | --- |
| 1 | `mlflow.Client.GetRun` | Green against `httptest`; 404 error mapped to `APIError` |
| 2 | Port `Status` tool onto the client | No inline `http.Get` left in `main.go` |
| 3 | `mlflow://runs/{run_id}` template | Inspector shows it; unknown run id gives a clean error, not a crash |
| 4 | `get_run` tool | Returns a summary; measured token cost vs raw JSON |
| 5 | `SearchRuns` + pagination | `iter.Seq2` or a loop; `page_token` exercised in a test |
| 6 | `compare-runs` prompt | Appears in Inspector; composes `get_run` |
| 7 | Negative-path test | MLflow stopped → tool returns an error and the **server stays alive** |

## 9. Non-negotiables for agents

- Do not add third-party dependencies without asking.
- Do not add tools that wrap a single fixed API call with no judgment or shaping.
- Do not move logic from `internal/mlflow` into handlers, or vice versa.
- Do not silence errors (`_ = err`) or convert them to empty results.
- Do not add global mutable state; the client is constructed once in `main` and passed down.
- Do not write a README that claims more than the tests prove.
- Keep commits single-purpose.

## 10. Dev loop

```bash
# MLflow fixture (repo root)
source .venv/bin/activate
mlflow server --backend-store-uri sqlite:///mlflow.db --port 5000

# build, test
go build ./...
go test ./... -race

# inspect the server by hand
npx @modelcontextprotocol/inspector go run ./cmd/mlflow-mcp
```

Unit tests must never need a live MLflow server. Anything requiring one goes behind an env var.

## 11. Repo hygiene

- Never commit compiled binaries.
- Never commit `*.db`, `.venv/`, `mlflow.db`, or local MCP client configs.
- `go.mod` pins the SDK; state the version in the README. The protocol moves fast.
- License: MIT.

## 12. Background the code won't tell you

- **The panic incident.** The first version called `panic(err)` inside the tool handler. MLflow's `/health` returns `text/plain OK`, JSON decoding failed, the process died, and the tool vanished from the client's catalog with no visible error. This is why rule 5.1 exists and why `validate`-style negative tests matter more than happy-path ones.
- **Why the official SDK and not `mark3labs/mcp-go`.** The official SDK tracks the spec first and its schema-from-structs design is part of what is being learned. The tradeoff (fewer tutorials, more source reading) is accepted deliberately.
- **Why `mlflow://runs/{id}` is a template.** Thousands of runs; `resources/list` must not enumerate them.
- **Why both a resource and a `get_run` tool for the same data.** Clients let the *user* attach resources; the *model* calls tools. Agents asked to compare runs need the tool.

## 13. Open questions

- Which MLflow version to target (2.x vs 3.x endpoints for logged models)?
- Whether `compare_runs` stays a tool or lives only in the prompt.
- Whether to expose `mlflow://experiments/{id}/runs` as a resource once the run list is large.
- Deployment shape for the practice deploy: Docker + Streamable HTTP, OAuth out of scope for now.