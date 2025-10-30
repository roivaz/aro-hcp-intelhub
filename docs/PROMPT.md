## Tool Selection and MCP Knowledge Fabric

**CRITICAL DIRECTIVE: YOU MUST PRIORITIZE THE MCP KNOWLEDGE FABRIC.**

The primary source of truth for operational issues, historical context, and design decisions is in the MCP tools (`search_docs`, `search_prs`). Direct code investigation (`grep`, `read_file`, `codebase_search`) is a **secondary fallback**, to be used only after the knowledge fabric has been consulted.

**ANTI-PATTERN #1: Do NOT start with `codebase_search` or `grep` for operational issues.**
- **Reason**: The "why" of a change and the "how to fix it" live in PRs and docs, not just in the code.
- **Mandatory Action**: Always start with the workflow defined below.

---

### Guiding Principles of Investigation
- **Start broad** (MCP search) → **Get specific** (codebase_search, grep, read_file)
- **Understand context** (MCP tools) → **Verify implementation** (code tools)
- **Learn from history** (search_prs) → **Apply to present** (current codebase)

---

### Mandatory Investigation Workflow

**For any task involving debugging, incident response, or understanding unexpected behavior, you MUST follow this exact sequence:**

**Phase 1: Context Gathering (Non-Code First)**
1.  **Search for historical context and recent changes.** This is the most critical first step. Use `mcp_aro-hcp-embeddings_search_prs`. Formulate a query about the *symptoms or components involved*.
    - *Example Query:* "cluster creation API validation changes"
2.  **Search for architectural and design documentation.** After checking for recent changes, use `mcp_aro-hcp-embeddings_search_docs` to understand how the system is *supposed* to work. This is for building a mental model, not for finding specific solutions.
    - *Example Query:* "cluster creation flow architecture"
3.  **Analyze the results from steps 1 & 2.**
    - If you find relevant PRs, your investigation MUST start there. Use `get_pr_details` for specifics. This is your primary lead.
    - If you find relevant docs, use them to build your understanding of the system's design.
    - If you find nothing, you must explicitly state: "No relevant documentation or recent PRs found related to the issue. Proceeding to code-level analysis."

**Phase 2: Code-Level Investigation (Only after Phase 1)**
4.  **Iterative Deepening**: If a tool call (like `get_pr_details`) reveals new, specific technical terms, component names, or concepts, you MUST perform at least one more `search_docs` or `search_prs` cycle using these new terms. Do not move to code-level investigation while new contextual leads are still being uncovered.
5.  **Targeted Code Analysis**: Once Phase 1 is complete, you may proceed with code-level tools. Use the right tool for the job: `codebase_search` for semantic "how does it work" questions, `grep` for exact symbol lookups, and `read_file` when you know the specific file to examine. Your reasoning MUST be based on hypotheses formed from the initial problem description.
6.  **Principle of Context Re-Evaluation**: If, during code-level investigation, you discover a new, high-level concept (e.g., an unfamiliar service name, a feature flag, a core architectural component), you MUST treat this as a trigger to re-engage the knowledge fabric. You should temporarily pause your code analysis and use `search_docs` and `search_prs` with this new term to seek high-level context before diving deeper into its implementation. The default is always to seek context before code.