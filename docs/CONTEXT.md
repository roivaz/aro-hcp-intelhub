# ARO-HCP Release & Incident Intelligence

### Purpose
ARO-HCP Release & Incident Intelligence unifies code changes, deployments, and operational knowledge into a change‑aware knowledge fabric accessible via MCP. It answers what changed, where, why, and what to do next—cutting time‑to‑clarity for SREs and engineers during releases and incidents.

## Architecture Overview
- `cmd/ingest`: orchestrates PR fetching, diff analysis, and embedding storage.
- `cmd/mcp-server`: JSON-RPC MCP server exposing `search_prs`, `get_pr_details`, `trace_images`, and `search_docs`.
- `cmd/dbstatus`: connectivity checker used by `make db-status`.
- `cmd/dbctl`: centralized database control CLI (`init`, `migrate`, `status`, `verify`, `recreate`).
- `internal/ingestion/diff`: map/reduce diff analyzer using Ollama (`phi3`), recursive chunking, token estimation.
- `internal/ingestion/embeddings`: talks to Ollama (`nomic-embed-text`) and persists vectors (pgvector).
- `internal/tracing`: Skopeo-backed inspector that maps image digests to source commits.
- `internal/db`: PostgreSQL access via Bun (pgvector enabled).
- `internal/db/migrate`: migration helpers + schema checks used by `dbctl` and ingest startup.
- `internal/gitrepo`: git CLI wrapper (ensure/fetch/worktree/headsha/diff/list/show) used by diff analyzer, tracer, and docs.
- `config-go.env`: central configuration consumed by binaries and container image.
- `cmd/ingest docs`: Markdown docs ingestion (chunk → embed → store in `documents`).
- `cmd/dbctl`: dedicated database control CLI (init/migrate/status/verify/recreate).

## Data Flow
1. **GitHub Fetching (Incremental)**: Ingest fetches merged PR metadata from GitHub API, scanning newest pages first and stopping once cached PRs are encountered. Fetches up to `GITHUB_FETCH_MAX` new PRs per run.
2. **Two-Phase Ingestion Architecture**:
   - **CACHE mode**: Rapidly fetches and stores PR metadata only (no embeddings/analysis). Can ingest thousands of PRs in seconds.
   - **PROCESS mode**: Sequentially processes unprocessed PRs from DB (embedding generation + diff analysis).
   - **FULL mode**: Combines both phases (cache then process) for convenience.
3. Local git clone (PR ref workflow) produces diffs; analyzer chunks/filters to avoid generated files.
4. Map stage calls Ollama per chunk; reduce stage synthesizes summary; results stored with token statistics.
5. Embeddings generated via Ollama embeddings endpoint and saved in `pr_embeddings` table.
6. MCP server queries embeddings DB (only processed PRs with `embedding IS NOT NULL`) and routes tool invocations; `trace_images` shells out to Skopeo.
7. Documentation ingestion (`ingest docs`) clones public/private repos to cache, chunks Markdown with langchaingo, embeds with `nomic-embed-text`, and stores chunks in `documents` (pgvector). `search_docs` embeds user query and searches `documents`; when `include_full_file` is true, returns the full file content from local cache.

## Key Decisions
- **Merge-commit diff strategy** (merge^1 vs merge) for closed PR accuracy.
- **Recursive diff chunking** with `langchaingo/textsplitter` to stay within LLM context.
- **Two-phase ingestion**: Decouple fast GitHub caching from expensive LLM processing to handle rate limits efficiently.
- **Incremental-only fetching**: Always resume from latest DB timestamp, eliminating complex batch/direction logic.
- **Sequential processing**: Single-worker processing for embedding/diff analysis (hardware constraints).
- **Nullable embeddings**: `pr_embeddings.embedding` and `processed_at` are nullable to distinguish cached vs. processed PRs.
- **Shared `aro_hcp_repo_path`** for diff analyzer and tracer to keep clone management consistent.
- **Skopeo CLI usage** avoids Docker-in-Docker and supports registry auth via pull-secret file.
- **Go-based Makefile & Dockerfile** replace Python tooling; distroless image ships static binaries.

## Tooling & Operational Notes
- `make compose-up` starts a local Postgres (pgvector) via docker-compose; `make compose-down` stops it; `make compose-db-bootstrap` initializes migrations with `dbctl`.
- `make db-bootstrap`, `make db-status`, `make db-verify` drive schema init and checks via `dbctl`.
- `make run-ingest`, `make run-mcp` for local workflows once Postgres is up.
- `make container-build` builds Go multi-stage image; `make kind-create` boots kind + cloud-provider-kind and preloads the image.
- MCP endpoint: `http://host:8000/mcp/jsonrpc`; update Cursor/Claude configs accordingly.
- Ensure Ollama models (`phi3`, `nomic-embed-text`) are available; set `ollama_url` when using remote GPU.
- Provide `pull_secret` when tracing images that live in private registries.

## Configuration
**Execution Modes** (set via `EXECUTION_MODE`):
- `FULL` (default): Fetch from GitHub + process (embeddings + diff analysis) - convenience mode
- `CACHE`: Fast metadata-only fetching from GitHub (respects rate limits, no LLM calls)
- `PROCESS`: Process cached PRs sequentially (embeddings + diff analysis)

**Key Environment Variables**:
- `GITHUB_FETCH_MAX`: Maximum PRs to fetch from GitHub per run (default: 100)
- `MAX_PROCESS_BATCH`: Maximum PRs to process from DB per run (default: 100)
- `DIFF_ANALYSIS_ENABLED`: Enable LLM-based diff analysis (default: false)

**Recommended Workflow**:
1. Use `EXECUTION_MODE=CACHE` to rapidly build PR cache (thousands in seconds)
2. Use `EXECUTION_MODE=PROCESS` to incrementally process cached PRs when resources available
3. Use `EXECUTION_MODE=FULL` for convenience (combines both phases)

## Recent Improvements (Session Notes)
### October 2025 - Ingestion Architecture Refactor
**Problem Solved**: Original ingestion was counting skipped PRs toward the limit, leading to no new PRs being ingested.

**Major Changes**:
1. **Fixed GitHub Fetching Logic**: Always fetch 100 PRs per page from GitHub API, filter existing PRs client-side, only count new PRs toward `GITHUB_FETCH_MAX`.
2. **Unified Ingestion Modes**: Eliminated separate incremental/batch logic; all modes now work incrementally (resume from latest DB timestamp or config start date).
3. **Optimized GitHub API Sorting**: Align GitHub sort order with fetch direction (ASC for "onwards") to minimize pages fetched.
4. **Two-Phase Architecture**: Split caching (fast) from processing (expensive LLM operations):
   - Added `EXECUTION_MODE` configuration (FULL, CACHE, PROCESS)
   - Added nullable `embedding` and `processed_at` fields to `pr_embeddings` table
   - Added DB methods: `GetUnprocessedPRs()`, `CountUnprocessedPRs()`, `UpdatePRProcessing()`
5. **Configuration Cleanup**: Renamed for clarity and consistency:
   - `INGESTION_LIMIT` → `GITHUB_FETCH_MAX`
   - `INGESTION_MODE` → removed (always incremental now)
   - `INGESTION_START_DATE` → `GITHUB_FETCH_START_DATE`
   - `BATCH_MODE_DIRECTION` → removed (always "onwards" now)
   - `PROCESS_LIMIT` → `MAX_PROCESS_BATCH`
   - `PROCESS_WORKERS` → removed (sequential processing only)
6. **Code Simplification**: Reduced `generator.go` from 442 lines to 290 lines (34% reduction):
   - Removed batch mode entirely (CACHE mode is fast enough)
   - Consolidated duplicate fetching logic into shared `fetchNewPRs()` helper
   - Made `RunFull()` simply call `RunCache()` + `RunProcess()` (composition over duplication)
   - Eliminated dead code and redundant safety checks

**Performance Impact**: CACHE mode can now ingest 5000 PRs in seconds, making the entire repo history cacheable in one run.

### October 2025 - Repo Ops Refactor & Docs Ingestion
- Added `internal/gitrepo` and refactored tracer and diff analyzer to use it (shelling out to git for clone/fetch/worktree/diff and read-only file ops).
- Added `ingest docs` command: Markdown-aware chunking via langchaingo, embeddings via Ollama, persisted to `documents` (VECTOR(768)).

### October 2025 - DB Command, Local DB, and Docs Search Tool
- Introduced `cmd/dbctl` for centralized DB bootstrap and migrations (init/migrate/status/verify/recreate).
- Added local Postgres (pgvector) via docker-compose with Makefile helpers.
- Implemented `search_docs` MCP tool:
  - Inputs: `query`, optional `limit`, `component`, `repo`, `include_full_file`.
  - Behavior: embeds the query, searches `documents` by cosine distance; when `include_full_file` is true, returns the complete file content from local cache at the matched commit.
- Added tool descriptions in the MCP server so AI agents properly discover available tooling.

### October 2025 - Trace Images Simplification & CLI
- Added `cmd/trace-images` CLI sharing the MCP `trace_images` flow, making local traces easy to run against any commit/environment.
- Folded the tracer into `internal/traceimages`, removed the separate `internal/tracing` package, and reused the same cache-aware service everywhere.



