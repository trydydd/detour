# 03. HTTP Proxy Request Handling

## Topic Statement

Accept incoming HTTP requests, validate them, route to appropriate backend, and apply transformations based on routing decision.

## Scope

**In-scope:**
- HTTP endpoint registration and request dispatching
- Request body validation for Messages API
- Model extraction and routing decision
- Request transformation before forwarding
- Response delivery to client

**Boundaries:**
- Input: HTTP requests from Claude Code client
- Output: HTTP responses to client
- External: Local inference server or Anthropic API as upstream

## Data Contracts

### Supported Endpoints

| Path | Method | Description |
|------|--------|-------------|
| `/health` | GET | Health check endpoint |
| `/v1/messages` | POST | Messages API endpoint |
| `/v1/models` | GET | Models list endpoint (passthrough) |
| `/*` | Any | Catch-all passthrough to Anthropic |

### Request Body Shape (Messages API)

```json
{
  "model": "string",
  "messages": "array",
  "max_tokens": "integer",
  "thinking": "object (optional)",
  "stream": "boolean (optional)"
}
```

### Maximum Request Size

- Maximum request body: 10 MiB (10,485,760 bytes)
- Request exceeding limit returns 413 error

## Behaviors

### Endpoint Registration

Proxy registers handlers in order:
1. `/health` - health check
2. `/v1/messages` - Messages API with routing logic
3. `/v1/models` - models list passthrough
4. `/` - catch-all passthrough

### Health Check Handler

Returns JSON with status and version:
```json
{
  "status": "ok",
  "version": "0.1.0"
```

### Messages Handler Sequence

1. Read request body with size limit enforcement
2. Parse JSON and extract `model` field
3. Validate `model` field is present and non-empty
4. Call routing function with model name and local alias
5. Apply transformations based on routing decision
6. Forward request to appropriate upstream
7. Return upstream response to client

### Request Validation Errors

| Condition | HTTP Status | Error Type | Message |
|-----------|-------------|------------|---------|
| Body read failure | 400 | invalid_request | "could not read request body" |
| Body exceeds limit | 413 | invalid_request | "request body too large" |
| Invalid JSON | 400 | invalid_request | JSON parse error details |
| Missing model field | 400 | invalid_request | "missing required field: model" |

### Local Route Transformations

When routing to local inference server:

1. Remove `thinking` field from request body
2. If `Anthropic-Beta` header present, remove tokens containing "thinking"
3. Forward modified request to local upstream

### Passthrough Route Behavior

When routing to Anthropic API:

1. Forward request unchanged
2. Preserve all original fields including `thinking`
3. Preserve all original headers including `Anthropic-Beta`

### Models Handler Behavior

Forward `/v1/models` request to Anthropic API unchanged.

### Passthrough Catch-all Handler

Forward any unrecognized path to Anthropic API with original URI preserved.

## State Transitions

| Request | Transformation | Target |
|---------|----------------|--------|
| `/health` | None | Local response |
| `/v1/messages` + local model | Strip thinking, filter beta | Local upstream |
| `/v1/messages` + other model | None | Anthropic upstream |
| `/v1/models` | None | Anthropic upstream |
| `/*` (other) | None | Anthropic upstream |

## Notable Behaviors

1. Thinking field removal prevents local servers from generating blocks with invalid signatures that would break subsequent passthrough requests
2. Beta header filtering removes thinking-related tokens to prevent compatibility issues
3. Catch-all handler ensures any unrecognized paths still reach Anthropic
