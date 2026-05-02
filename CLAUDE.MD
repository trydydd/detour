## Code Exploration Policy

Always use jCodemunch-MCP tools for code navigation. Never fall back to Read, Grep, Glob, or Bash for code exploration.

**Start any session:**
1. `resolve_repo { "path": "." }` — confirm the project is indexed. If not: `index_folder { "path": "." }`
2. `suggest_queries` — when the repo is unfamiliar

**Finding code:**
- symbol by name → `search_symbols` (add `kind=`, `language=`, `file_pattern=` to narrow)
- string, comment, config value → `search_text` (supports regex, `context_lines`)
- database columns (dbt/SQLMesh) → `search_columns`

**Reading code:**
- before opening any file → `get_file_outline` first
- one or more symbols → `get_symbol_source` (single ID → flat object; array → batch)
- symbol + its imports → `get_context_bundle`
- specific line range only → `get_file_content` (last resort)

**Repo structure:**
- `get_repo_outline` → dirs, languages, symbol counts
- `get_file_tree` → file layout, filter with `path_prefix`

**Relationships & impact:**
- what imports this file → `find_importers`
- where is this name used → `find_references`
- is this identifier used anywhere → `check_references`
- file dependency graph → `get_dependency_graph`
- what breaks if I change X → `get_blast_radius` (add `include_depth_scores=true` for layered risk, `include_source=true` for fix-ready context)
- what symbols actually changed since last commit → `get_changed_symbols`
- find unreachable/dead code → `find_dead_code`
- most important symbols by architecture → `get_symbol_importance`
- class hierarchy → `get_class_hierarchy`
- callers/callees of a symbol → `get_call_hierarchy`
- high-risk symbols (complexity × churn) → `get_hotspots`
- related symbols → `get_related_symbols`
- diff two snapshots → `get_symbol_diff`
- symbols by decorator → `search_symbols(decorator="...")` or `get_blast_radius(decorator_filter="...")`

**Session awareness:**
- starting a task → `plan_turn` (confidence + recommended symbols/files)
- session history → `get_session_context`
- after editing → `register_edit` (invalidates caches)

**Retrieval with token budget:**
- best-fit context for a task → `get_ranked_context` (query + token_budget)
- bounded symbol bundle → `get_context_bundle` (add token_budget= to cap size)

**After editing a file:** `index_file { "path": "/abs/path/to/file" }` to keep the index fresh.

## Detour Proxy Notes

The `/v1/models` endpoint on the proxy passes through to Anthropic and cannot be tested with `curl` without valid Anthropic auth credentials. Do not suggest testing `/v1/models` with curl or curl alternatives—this will always fail with an authentication error. The proxy endpoint is only testable by Claude Code itself, which has internal authentication credentials.