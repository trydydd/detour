# 15. Health and Models Endpoints

## Topic Statement

Provide health check and model listing endpoints for proxy status verification and upstream model discovery.

## Scope

**In-scope:**
- `/health` endpoint implementation
- `/v1/models` endpoint implementation
- Response formats and status codes

**Boundaries:**
- Input: HTTP GET requests to endpoints
- Output: JSON responses with status or model information
- `/health` returns local proxy status
- `/v1/models` forwards to Anthropic API

## Data Contracts

### Health Endpoint Response

```json
{
  "status": "ok",
  "version": "0.1.0"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Always "ok" when endpoint is reachable |
| `version` | string | Proxy version string (e.g., "0.1.0") |

### Models Endpoint Response (from Anthropic)

```json
{
  "object": "list",
  "data": [
    {
      "id": "string",
      "object": "model",
      "created": number,
      "owned_by": "string"
    }
  ]
}
```

The response format matches the Anthropic API models endpoint format.

## Behaviors

### Health Endpoint

**Path:** `/health`
**Method:** GET

Behavior:
1. Accept GET request
2. Set `Content-Type` header to `application/json`
3. Encode and return health status object
4. Return HTTP 200 OK

**Request logging:** Not logged (handler not wrapped with `maybeLog`)

### Models Endpoint

**Path:** `/v1/models`
**Method:** GET

Behavior:
1. Accept GET request
2. Forward request to Anthropic API at `https://api.anthropic.com/v1/models`
3. Copy all response headers from upstream
4. Copy response body from upstream
5. Return upstream status code and body to client

**Request logging:** Wrapped with `maybeLog` - logged when `DETOUR_LOG` is enabled

### Error Handling

| Condition | HTTP Status | Response |
|-----------|-------------|----------|
| Non-GET method on /health | 405 | Method not allowed (default Go behavior) |
| Non-GET method on /v1/models | Pass through | Forwarded to Anthropic |
| Upstream unavailable | 502 | Proxy error (from forward layer) |

## State Transitions

| Endpoint | Method | Action |
|----------|--------|--------|
| `/health` | GET | Return local status JSON |
| `/v1/models` | GET | Forward to Anthropic |
| Any other path | Any | Handled by catch-all passthrough |

## Notable Behaviors

1. **Health endpoint is local**: Does not require upstream connectivity; verifies proxy is running

2. **No authentication required**: Health endpoint can be checked without API keys

3. **Version reporting**: Health endpoint exposes proxy version for debugging and compatibility checking

4. **Models passthrough**: `/v1/models` is always forwarded to Anthropic, never to local inference servers

5. **Logging exemption**: Health endpoint is not wrapped with request logging middleware, reducing noise in logs

## Rationale

These endpoints serve distinct purposes:

**Health endpoint:**
- Allows monitoring systems to verify proxy availability
- Provides version information for debugging
- Can be used to verify proxy startup completion
- Does not require external connectivity, making it a true health check

**Models endpoint:**
- Provides compatibility with clients that list available models
- Forwards to Anthropic to show all available models (both local alias and passthrough models)
- Enables clients to discover model options programmatically

## Examples

### Health Check Request

```
GET /health HTTP/1.1
Host: 127.0.0.1:8888
```

Response:
```
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok","version":"0.1.0"}
```

### Models List Request

```
GET /v1/models HTTP/1.1
Host: 127.0.0.1:8888
X-Api-Key: sk-ant-xxx
```

Response (example from Anthropic):
```
HTTP/1.1 200 OK
Content-Type: application/json

{
  "object": "list",
  "data": [
    {
      "id": "claude-3-5-sonnet-20241022",
      "object": "model",
      "created": 1691606400,
      "owned_by": "anthropic"
    },
    {
      "id": "claude-3-opus-20240229",
      "object": "model",
      "created": 1709251200,
      "owned_by": "anthropic"
    }
  ]
}
```
