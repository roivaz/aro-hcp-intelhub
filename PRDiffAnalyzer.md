# PRDiffAnalyzer Flow Overview

This document describes the end-to-end processing flow executed by the `PRDiffAnalyzer` class (`pr_diff_analyzer.py`). The analyzer converts a merged pull request into a concise, structured summary suitable for embedding and downstream semantic search.

## High-Level Responsibilities

1. **Initialization**: Configure the Ollama-backed LLM, token estimator, diff splitters, and generated-file filters.
2. **Diff Acquisition**: Fetch the consolidated diff (`base..head`) for the supplied PR metadata.
3. **Filtering & Chunking**: Remove low-value/generated files, then split remaining diff content into token-aware chunks per file.
4. **Map Stage**: Run the map prompt over each chunk to extract factual bullet points.
5. **Reduce Stage**: Combine map outputs with PR metadata to produce a final, structured report.

## Detailed Flow

```mermaid
flowchart TD
    A["analyze_pr_diff(pr_data)"] --> B{Repo available?}
    B -- "no" --> B1["Raise RuntimeError"]
    B -- "yes" --> C["get_pr_consolidated_diff"]
    C --> D{Diff content present?}
    D -- "no" --> D1["Fail: No diff content"]
    D -- "yes" --> E["_split_diff_into_files"]
    E --> F["_filter_generated_files"]
    F --> G{Files remaining?}
    G -- "no" --> G1["Fail: All files filtered"]
    G -- "yes" --> H["_build_documents"]
    H --> H1["_annotate_chunk"]
    H1 --> H2["_estimate_tokens"]
    H2 -- "fits" --> H3["Collect Document"]
    H2 -- "too large" --> H4["_split_large_chunk"]
    H4 --> H1
    H3 --> I["Log diagnostics"]
    I --> J["Map phase loop"]
    J --> J1["map_chain.invoke"]
    J1 --> J2["Collect map results"]
    J2 --> K["Combine map summaries"]
    K --> L["reduce_chain.invoke"]
    L --> M["Return PRAnalysis (success)"]

    subgraph Failure_Paths
        direction TB
        B1
        D1
        G1
        M1["Exception handler<br/>Return PRAnalysis (failure)"]
    end

    J1 -- "exception" --> M1
    L -- "exception" --> M1
```


### Recursive Chunk Splitting Detail

```mermaid
flowchart TD
    A["Oversized chunk"] --> B["_split_large_chunk"]
    B --> C["Initialize queue = [chunk]"]
    C --> D["Pop next segment"]
    D --> E["Increment safety counter"]
    E --> F{Safety limit exceeded?}
    F -- "yes" --> F1["Warn & append remaining queue"]
    F1 --> K["Return result chunks"]
    F -- "no" --> G{Segment within token limit?}
    G -- "yes" --> H["Append to results"]
    H --> I{Queue empty?}
    G -- "no" --> J["RecursiveCharacterTextSplitter"]
    J --> J1{Multiple parts produced?}
    J1 -- "no" --> J2["Warn & append segment as-is"]
    J2 --> I
    J1 -- "yes" --> J3["Prepend new parts to queue"]
    J3 --> I
    I -- "no" --> D
    I -- "yes" --> K["Return result chunks"]
```

## Method Interactions

- `__init__`
  - Configures `ChatOllama`, token estimation (`tiktoken` when available), diff splitters, and ignore patterns.
  - Warns if instantiated without a repository path.

- `_build_ignore_patterns` / `_should_ignore_file`
  - Provide repository-specific heuristics to drop generated or low-value files before analysis.

- `_split_diff_into_files`
  - Breaks the consolidated diff at each `diff --git` header, tracking the new file path.

- `_build_documents`
  - Applies token estimation per file diff; oversized chunks are split via `_split_large_chunk` and annotated with file/path metadata for downstream prompts.

- `_split_large_chunk`
  - Recursively splits text until each segment falls under the token threshold, using `RecursiveCharacterTextSplitter` with diff-aware separators.

- `analyze_pr_diff`
  - Orchestrates the full pipeline: diff retrieval → filtering → chunking → map loop → reduce synthesis.
  - Emits rich console diagnostics (file skip reasons, token stats, map previews) aiding manual validation.

## Diagnostics & Logging

- Console output summarizes included vs. filtered files, chunk counts, and estimated token sizes.
- Map phase logs per-chunk previews/responses (limited to the first few chunks) for debugging prompt quality.
- Errors surface with `analysis_successful=False` while retaining context in `failure_reason`.


## Diff Splitting & Filtering Flow

```mermaid
flowchart TD
    A["Consolidated diff<br/>(base..head)"] --> B["_split_diff_into_files"]

    subgraph FileSplitting
        direction TB
        B --> B1["Regex split at 'diff --git'"]
        B1 --> B2["Extract file path (prefer new)"]
        B2 --> B3["Collect (file_path, chunk) pairs"]
    end

    B3 --> C["_filter_generated_files"]

    subgraph GeneratedFilter
        direction TB
        C --> C1{Matches ignore pattern?}
        C1 -- "yes" --> C2["Record skip (path, reason)"]
        C2 --> C4["Skipped set"]
        C1 -- "no" --> C3["Keep file chunk"]
        C3 --> C5["Included set"]
    end

    C5 --> D{Any included files?}
    D -- "no" --> D1["Fail: all files filtered"]
    D -- "yes" --> E["_build_documents"]

    subgraph DocumentBuilder
        direction TB
        E --> E1["Iterate included files"]
        E1 --> E2["Estimate tokens"]
        E2 --> E3{≤ token limit?}
        E3 -- "yes" --> E4["Use single chunk"]
        E3 -- "no" --> E5["_split_large_chunk"]
        E5 --> E6["Recursive diff-aware split"]
        E6 --> E4
        E4 --> E7["Annotate chunk metadata"]
        E7 --> E8["Wrap as LangChain Document"]
        E8 --> E9["Update stats (max/median tokens)"]
    end

    E9 --> F["Return documents + stats"]

    C4 -.-> G["Diagnostics: log skipped files"]
    F --> H["Diagnostics: console summary"]
```


