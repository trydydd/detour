# Model Name Extraction and Routing

## Topic Statement

This spec defines how the proxy extracts the model name from requests and uses it to determine routing destinations.

## Scope

**In-scope:**
- Model field extraction from JSON request body
- Routing decision logic
- Model name comparison behavior

**Boundaries:**
- Does not cover configuration of model names (see spec 01)
- Does not cover the actual forwarding (see spec 04)
- Does not cover transformation after routing (see spec 20)

## Data Contracts

### Request Body Structure

The `/v1/messages` endpoint expects JSON with the following structure:

```json
{
  "model": "<string>",
  "messages": [...],
  "max_tokens": <integer>,
  ...
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| model | string | Yes | Model identifier used for routing |
| messages | array | Yes | Message history (not inspected by router) |
| max_tokens | integer | No | Response length limit (not inspected by router) |

### Routing Output

| Value | Meaning | Destination |
|-------|---------|-------------|
| "local" | Model matches configured alias | `LocalUpstreamURL` |
| "passthrough" | Model does not match | `AnthropicUpstreamURL` |

## Behaviors

### Model Extraction

1. **JSON Parsing**: The request body is parsed as JSON into a generic object structure.

2. **Field Lookup**: The `model` field is extracted from the parsed JSON.

3. **Value Extraction**: The model value is extracted as a string.

4. **Error Handling**:
   - If JSON parsing fails: Return 400 with error message from parser
   - If `model` field is missing: Return 400 with message "missing required field: model"
   - If `model` is not a string: Return 400 with parse error details

### Routing Decision

1. **Comparison**: The extracted model name is compared against the configured `ModelName` using exact string equality.

2. **Decision**:
   - If `model == ModelName`: Route = "local"
   - If `model != ModelName`: Route = "passthrough"

3. **No Fallback**: There is no default routing; every request is explicitly routed to either local or passthrough.

### Routing Table

| Configured ModelName | Request Model | Route | Reason |
|---------------------|---------------|-------|--------|
| "red" | "red" | local | Exact match |
| "red" | "claude-opus-4-7" | passthrough | No match |
| "red" | "claude-sonnet-4-6" | passthrough | No match |
| "local-model" | "anything-else" | passthrough | No match |
| "" (empty) | Any | passthrough | Empty string never matches |

## State Transitions

| Input Model | Configured Name | Routing Result |
|-------------|-----------------|----------------|
| "red" | "red" | local |
| "red" | "blue" | passthrough |
| "claude-opus-4-7" | "red" | passthrough |
| "" | "red" | passthrough |
| null | "red" | 400 error (missing field) |

## Notable Behaviors

1. **Exact String Match Only**: Routing uses simple string equality. No partial matching, prefix matching, or pattern matching is performed.

2. **Case Sensitive**: Model name comparison is case-sensitive. "Red" does not match "red".

3. **Single Alias**: Only one model name can be configured as the local alias. All other models route to Anthropic.

4. **No Model Validation**: The proxy does not validate whether the model name is valid or known. Any string value is accepted.

5. **No Model List**: The proxy does not maintain a list of available models. Model availability is determined by the upstream services.

## Error Conditions

| Condition | HTTP Status | Error Type | Message |
|-----------|-------------|------------|---------|
| Invalid JSON | 400 | invalid_request | JSON parse error details |
| Missing model field | 400 | invalid_request | "missing required field: model" |
| Non-string model | 400 | invalid_request | Parse error details |

## Rationale

The simple string matching design:

1. **Predictability**: Exact matching eliminates ambiguity about which model will be routed where.

2. **Simplicity**: Minimal code reduces bug surface and makes behavior easy to understand.

3. **Flexibility**: Users can configure any model name as their local alias without restrictions.

4. **Explicitness**: There is no implicit behavior; users must explicitly configure which model routes locally.

## Examples

### Matching Model

**Configuration:**
```json
{
  "model_name": "red",
  "model_api": "http://localhost:8000"
}
```

**Request:**
```json
{
  "model": "red",
  "messages": [{"role": "user", "content": "Hello"}]
}
```

**Result:** Routes to `http://localhost:8000/v1/messages`

### Non-Matching Model

**Configuration:**
```json
{
  "model_name": "red",
  "model_api": "http://localhost:8000"
}
```

**Request:**
```json
{
  "model": "claude-opus-4-7",
  "messages": [{"role": "user", "content": "Hello"}]
}
```

**Result:** Routes to `https://api.anthropic.com/v1/messages`

### Missing Model Field

**Request:**
```json
{
  "messages": [{"role": "user", "content": "Hello"}]
}
```

**Result:** HTTP 400
```json
{
  "type": "error",
  "error": {
    "type": "invalid_request",
    "message": "missing required field: model"
  }
}
```
