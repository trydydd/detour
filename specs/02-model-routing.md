# 02. Model Routing Logic

## Topic Statement

Determine whether a model request should be routed to local inference server or passthrough to Anthropic API.

## Scope

**In-scope:**
- Model name matching against configured local alias
- Routing decision output

**Boundaries:**
- Input: model name from request, configured local model alias
- Output: routing decision ("local" or "passthrough")

## Data Contracts

### Input

| Field | Source | Description |
|-------|--------|-------------|
| `model` | Request body field | Model name from incoming API request |
| `localName` | Configuration | Configured local model alias |

### Output

| Value | Meaning |
|-------|---------|
| `"local"` | Route to configured local inference server |
| `"passthrough"` | Route to Anthropic API |

## Behaviors

### Routing Decision

Single rule determines routing:

- If `model` equals `localName` exactly (string comparison), return `"local"`
- For all other cases, return `"passthrough"`

## State Transitions

| Input Model | Local Alias | Routing Decision |
|-------------|-------------|------------------|
| "red" | "red" | local |
| "claude-opus-4-7" | "red" | passthrough |
| "claude-sonnet-4-6" | "red" | passthrough |
| "claude-haiku-4-5-20251001" | "red" | passthrough |
| "any-unknown-model" | "red" | passthrough |

## Notable Behaviors

1. Exact string match only — no prefix matching, substring matching, or fuzzy matching
2. All non-matching models route to Anthropic, including official Claude model names
3. Multi-model workflows supported: different model names in same session route differently
