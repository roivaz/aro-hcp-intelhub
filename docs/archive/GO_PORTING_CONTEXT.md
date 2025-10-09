## Context for Rewriting ARO-HCP AI-Assisted Observability (Python → Go)

This document describes ONLY the current functionality implemented in Python so it can be ported to Go. Do not include any new features beyond what exists today.

### Project purpose (current)
- Batch process ARO‑HCP repository history to generate semantic embeddings for PRs using Ollama models; store vectors in PostgreSQL with pgvector.
- Expose semantic search and details via an MCP (Model Context Protocol) HTTP/SSE server for AI agents (Cursor/Claude) with tools like `search_prs`, `get_pr_details`, etc.
- Provide an image tracing tool that resolves deployed component images to source repository commits using container registry metadata.

### Current Python components (high level)
- `embedding_generator.py` — batch pipeline to fetch merged PRs, analyze diffs, generate embeddings with Ollama, and persist to Postgres/pgvector; idempotent with processing state.
- `pr_diff_analyzer.py` — diff analysis/summarization logic used by the embedder.
- `database_models.py` — SQLModel ORM models and DB interactions for embeddings and processing state.
- `mcp_server.py` — FastAPI MCP server exposing current tools over HTTP/SSE.
- `aro_hcp_image_tracer.py` — image tracing from deployment config → image digest → labels (`vcs-ref`) → source commit/repo.
- `manifests/config.env` — environment configuration used by both batch and server.

### Current MCP tools (as implemented today)
- `search_prs` — semantic search across PR embeddings with titles, authors, states, and merge dates.
- `get_pr_details` — returns PR metadata and full description/body with links.
- `trace_images` — returns image→source trace data (component, registry/repo/digest, source commit/repo) using registry labels.

### Database schema (current model summary)
- Postgres with pgvector.
- `pr_embeddings` (vectors for PRs): `pr_number`, `pr_title`, `pr_body`, `author`, `created_at`, `merged_at`, `state`, `embedding VECTOR(768)`.
- `processing_state`: last processed PR number/date for idempotency.

### Environment/config (current)
Keys loaded from `manifests/config.env`:
- Postgres: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`.
- Ollama: `OLLAMA_URL` (e.g., `http://localhost:11434`).
- PR processing limits: `MAX_PR_COUNT` (legacy) and `MAX_NEW_PRS_PER_RUN` (current, default: 100).
- Optional PR bounds: `PR_START_DATE` (ISO-8601; skip earlier PRs when batching).
- Ingestion mode: `INGESTION_MODE` (`INCREMENTAL` or `BATCH`; default: `INCREMENTAL`).
- Execution model/embedding model (used by Python): `EXECUTION_MODEL_NAME`, `EMBEDDING_MODEL_NAME`.
- Log level: `LOG_LEVEL`.

---

## Target Go module layout (no new features)

```text
go/
├── go.mod
├── Makefile
├── cmd/
│   ├── mcp-server/
│   │   └── main.go              # starts MCP HTTP/SSE server
│   ├── trace/
│   │   └── main.go              # CLI for image tracing (wraps tracer)
│   └── ingest/
│       └── main.go              # batch embedder (PRs/commits)
├── internal/
│   ├── config/                  # Cobra+Viper binding + envfile
│   │   ├── config.go            # Init(rootCmd), defaults, typed getters
│   │   └── keys.go              # const keys (OLLAMA_URL, POSTGRES_URL, ...)
│   ├── logging/
│   │   └── logging.go           # structured logger setup
│   ├── mcp/                     # minimal MCP JSON-RPC over HTTP + SSE
│   │   ├── protocol.go          # initialize, ping, tools/list, tools/call types
│   │   ├── server.go            # HTTP handlers, SSE, tool registry
│   │   └── tools/
│   │       ├── search_prs.go
│   │       ├── get_pr_details.go
│   │       └── trace_images.go
│   ├── ingestion/               # batch embedding generator
│   │   ├── generator.go         # PR/commit processing pipeline
│   │   ├── github.go            # go-github client helpers
│   │   ├── diff.go              # diff retrieval/splitting (if needed)
│   │   └── embeddings.go        # Ollama embeddings client
│   ├── db/                      # Bun ORM (pgx driver) with pgvector
│   │   ├── connect.go
│   │   ├── models.go            # Go structs mirroring current tables
│   │   └── migrate/             # migrations (atlas/goose) if needed
│   ├── tracing/                 # image tracing via skopeo
│   │   ├── tracer.go            # orchestrates ref building + labels → source
│   │   └── skopeo/
│   │       └── inspector.go     # shell-out wrapper for `skopeo inspect`
│   └── utils/
│       └── paths.go             # repo root/data/cache resolution
└── data/
    └── component_cards.yaml     # existing curated summaries (if present)
```

Notes:
- MCP server must expose the SAME tools and JSON shapes the Python server returns today.
- Batch embedder should generate the SAME vector dimension (768) and store identical columns.
- Tracer must preserve behavior of `trace_images` (labels → `vcs-ref`/source commit mapping).

---

## Python → Go mapping (modules/classes → packages)

| Python file/class (today) | Purpose | Go package/file |
|---|---|---|
| `embedding_generator.py` (orchestrator/functions) | PR fetch, diff analysis integration, embeddings generation, DB writes, idempotency | `internal/ingestion/generator.go`, `internal/ingestion/github.go`, `internal/ingestion/embeddings.go` |
| `pr_diff_analyzer.py` (functions/classes) | Diff summarization/chunking used by embedder | `internal/ingestion/diff.go` (helpers); optional small summarizer in `internal/ingestion/embeddings.go` if needed |
| `database_models.py` (SQLModel classes) | ORM models and DB I/O for `pr_embeddings`, `processing_state` | `internal/db/models.go` + `connect.go` (Bun ORM) |
| `mcp_server.py` (FastAPI) | MCP HTTP/SSE; tools routing; tool schemas | `internal/mcp/protocol.go`, `internal/mcp/server.go`, `internal/mcp/tools/*.go` |
| `aro_hcp_image_tracer.py` (classes: tracer, inspector, parser) | Image tracing: config → image digest → labels → source commit | `internal/tracing/tracer.go`, `internal/tracing/skopeo/inspector.go` |
| `manifests/config.env` | Configuration used by both batch and server | Loaded by Cobra/Viper via `internal/config` |

Where Python used classes, Go will primarily use packages and small structs with methods; keep responsibilities aligned.

---

## Required Go interfaces and contracts

### 1) Config (Cobra + Viper + light config package)

```go
// internal/config/keys.go
const (
    KeyPostgresURL = "postgres_url"
    KeyOllamaURL   = "ollama_url"
    KeyLogLevel    = "log_level"
    KeyAuthFile    = "auth_file"     // path to docker config.json for skopeo
    KeyCacheDir    = "cache_dir"
)

// internal/config/config.go
func Init(root *cobra.Command) {
    viper.SetEnvPrefix("AROHCP")
    viper.AutomaticEnv()
    _ = godotenv.Load("manifests/config.env")
    _ = viper.BindPFlags(root.PersistentFlags())
    setDefaults()
}

func setDefaults() {
    viper.SetDefault(KeyOllamaURL, "http://localhost:11434")
    viper.SetDefault(KeyLogLevel, "info")
    viper.SetDefault(KeyCacheDir, "ignore/cache")
}

func PostgresURL() string { return viper.GetString(KeyPostgresURL) }
func OllamaURL() string   { return viper.GetString(KeyOllamaURL) }
func AuthFile() string    { return viper.GetString(KeyAuthFile) }
func CacheDir() string    { return viper.GetString(KeyCacheDir) }
```

Root command wiring (example):
```go
var root = &cobra.Command{ Use: "mcp-server", RunE: run }

func init() {
    root.PersistentFlags().String("postgres-url", "", "Postgres DSN")
    root.PersistentFlags().String("ollama-url", "", "Ollama base URL")
    root.PersistentFlags().String("auth-file", "", "Path to docker authfile for skopeo")
    root.PersistentFlags().String("cache-dir", "", "Cache directory")
    config.Init(root)
}
```

### 2) Skopeo-based image inspector

```go
// internal/tracing/skopeo/inspector.go
type ImageInspector interface {
    Labels(ctx context.Context, ref string) (map[string]string, error)
    InspectRaw(ctx context.Context, ref string) ([]byte, error) // optional
}

type SkopeoInspector struct {
    AuthFile string // path to docker config.json / pull-secret
    Retries  int
}

// ref format: registry/repository@sha256:<digest>, passed to skopeo as docker://<ref>
```


### 3) Tracer orchestrator

```go
// internal/tracing/tracer.go
type Tracer struct {
    Inspector skopeo.ImageInspector
}

type Component struct {
    Name       string
    Registry   string
    Repository string
    Digest     string
    Labels     map[string]string
    SourceRepo string // from labels if present
    SourceSHA  string // from labels if present
}

type TraceResult struct {
    CommitSHA   string
    Environment string
    Components  []Component
    Errors      []string
}

func (t *Tracer) Trace(ctx context.Context, commitSHA, environment string) (TraceResult, error)
```

Behavior:
- Resolve components (name→registry/repo/digest) from ARO‑HCP deployment config at `(commitSHA, environment)` using the same logic as Python today.
- For each image ref, call `Inspector.Labels` and fill `SourceRepo`/`SourceSHA` when present.
- Return structured result compatible with the current `trace_images` tool response.

### 4) Embeddings (Ollama) for batch ingest

```go
// internal/ingestion/embeddings.go
type EmbeddingClient interface {
    Embed(ctx context.Context, model string, inputs []string) ([][]float32, error)
}

type OllamaClient struct { BaseURL string }
```

Behavior:
- Call Ollama embeddings endpoint for model `nomic-embed-text` (768‑dim vectors) to mirror Python’s stored dimension and similarity behavior.

### 5) DB layer (Bun ORM + pgvector)

Connect with pgx driver and enable pgvector extension:

```go
// internal/db/connect.go
sqlDB := sql.OpenDB(stdlib.GetConnector(connString))
db := bun.NewDB(sqlDB, pgdialect.New())
db.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
db.RegisterModel((*PREmbedding)(nil))
pgvector.Register(db) // github.com/uptrace/bun/extra/pgvector
```

```go
// internal/db/models.go (Bun ORM structs reflect current tables)
type PREmbedding struct {
    bun.BaseModel `bun:"table:pr_embeddings"`
    PRNumber      int             `bun:"pr_number,pk"`
    PRTitle       string          `bun:"pr_title"`
    PRBody        string          `bun:"pr_body"`
    Author        string          `bun:"author"`
    CreatedAt     time.Time       `bun:"created_at"`
    MergedAt      *time.Time      `bun:"merged_at"`
    State         string          `bun:"state"`
    Embedding     pgvector.Vector `bun:"embedding"` // vector(768)
}

type ProcessingState struct {
    LastProcessedPR int
    LastProcessedAt time.Time
}
```

Queries should reproduce the current Python semantics (cosine distance via `<=>`). Index creation uses HNSW/IVFFlat as configured in Postgres.

### 6) MCP server contracts (HTTP/SSE; JSON‑RPC)

Implement `initialize`, `ping`, `tools/list`, and `tools/call` over HTTP; SSE for streaming tokens as the Python server does. Each tool’s input/output schema must match current Python responses.

Tools to expose (no additions):
- `search_prs`, `get_pr_details`, `trace_images`.

Minimal registry (pseudocode):
```go
type Tool func(ctx context.Context, params json.RawMessage) (any, error)

var Tools = map[string]Tool{
    "search_prs":            searchPRs,
    "get_pr_details":        getPRDetails,
    "trace_images":          traceImages,
}
```

---

## Ingestion pipeline parity (behavioral spec)
1) Modes supported: `INCREMENTAL` and `BATCH`.
   - INCREMENTAL: fetch merged PRs newer than the high watermark `(date, pr_number)` stored in `processing_state`; sort ascending; stop at `MAX_NEW_PRS_PER_RUN`.
   - BATCH: fetch older merged PRs before a low watermark `(date, pr_number)` (or before `PR_START_DATE` if set); sort descending; stop at `MAX_NEW_PRS_PER_RUN`.
2) Fetch merged PRs from GitHub (public; unauthenticated acceptable). Respect pagination and item cap (`MAX_NEW_PRS_PER_RUN`).
3) Generate analysis text/chunks for PR diffs as in Python (simple chunking acceptable if exact prompts aren’t moved).
4) Generate embeddings via Ollama `nomic-embed-text` and store vectors into `pr_embeddings` (VECTOR(768)).
5) Maintain idempotency: update `processing_state` with the last processed PR number/date for INCREMENTAL mode.

## Non-goals for this porting task
- Do not add new MCP tools or features beyond the list above.
- Do not implement new retrieval sources or documentation ingestion.
- Do not change vector dimensions or database schema semantics beyond a direct port.

---

## Acceptance checklist for the Go port
- MCP server responds with the same tool list (`search_prs`, `get_pr_details`, `trace_images`) and validates existing clients (Cursor/Claude) without changes.
- `search_prs`/`get_pr_details` produce equivalent fields and links; similarity scores comparable to Python.
- Ingestion supports both `INCREMENTAL` and `BATCH` modes with equivalent watermark semantics and item caps; persists to Postgres with pgvector (768‑dim) and idempotency for incremental.
- `trace_images` returns the same fields using Skopeo‑based inspector; labels mapping to `vcs-ref`/source repo preserved.
- Config can be supplied via flags, env (`AROHCP_*`), or `manifests/config.env`.

---

## Go-side configuration keys and ingestion mode type

Add keys for limits and optional start date:

```go
// internal/config/keys.go (add)
const (
    KeyMaxNewPRsPerRun = "max_new_prs_per_run" // default 100
    KeyPRStartDate     = "pr_start_date"       // optional ISO-8601 string
)

// internal/config/config.go (defaults)
viper.SetDefault(KeyMaxNewPRsPerRun, 100)
```

Expose a small mode enum and function signature:

```go
// internal/ingestion/generator.go
type IngestMode string

const (
    ModeIncremental IngestMode = "INCREMENTAL"
    ModeBatch        IngestMode = "BATCH"
)

type Watermarks struct {
    HighDate   *time.Time // for incremental upper bound
    HighNumber *int       // for incremental upper bound
    LowDate    *time.Time // for batch lower bound
    LowNumber  *int       // for batch lower bound
}

func (g *Generator) FetchPRs(ctx context.Context, mode IngestMode, w Watermarks, maxItems int) ([]PR, error)
```

CLI flags (example) bound via Cobra/Viper:

```go
root.PersistentFlags().String("mode", "INCREMENTAL", "ingestion mode: INCREMENTAL|BATCH")
root.PersistentFlags().Int("max-new-prs", 100, "cap per run")
root.PersistentFlags().String("pr-start-date", "", "ISO date; batch lower bound")
_ = viper.BindPFlag(config.KeyMaxNewPRsPerRun, root.PersistentFlags().Lookup("max-new-prs"))
_ = viper.BindPFlag(config.KeyPRStartDate, root.PersistentFlags().Lookup("pr-start-date"))
```


