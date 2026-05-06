# Upstream Error Propagation

## Topic Statement

This spec defines how HTTP errors from upstream services (Anthropic API or local inference servers) are propagated to the client through the detour proxy.

## Scope

**In-scope:**
- Error responses from upstream HTTP requests
- Status code preservation
- Error body passthrough behavior
- Proxy-level error generation

**Boundaries:**
- Does not cover request validation errors (see spec 16)
- Does not cover thinking block transformation (see spec 05)
- Does not cover streaming error handling (see spec 12)

## Data Contracts

### Upstream Error Response Format

When upstream returns an error, the response is forwarded unchanged:

| Component | Description |
|-----------|-------------|
| HTTP Status Code | Preserved exactly as returned by upstream |
| Response Headers | All upstream headers copied to client response |
| Response Body | Raw body forwarded without modification |

### Proxy-Generated Error Format

When the proxy itself generates an error (not from upstream):

```json
{
  "type": "error",
  "error": {
    "type": "<error_category>",
    "message": "<human_readable_description>"
  }
}
```

**Error categories:**
- `invalid_request` - Client sent malformed or invalid request
- `proxy_error` - Proxy encountered internal failure

## Behaviors

### Upstream Error Forwarding (Both Local and Passthrough Routes)

1. Proxy sends HTTP request to upstream service
2. Upstream responds with any status code
3. If upstream returns successfully (connection established):
   - Copy all response headers from upstream to client
   - Write upstream status code to client response
   - Copy upstream response body to client unchanged
4. No transformation of error content occurs

### Proxy-Generated Errors

The proxy generates errors in these conditions:

**Invalid Request (400 Bad Request):**
- Condition: Request body fails JSON parsing
- Response: `{"type":"error","error":{"type":"invalid_request","message":"could not read request body"}}`

**Missing Model Field (400 Bad Request):**
- Condition: JSON body does not contain `model` field
- Response: `{"type":"error","error":{"type":"invalid_request","message":"missing required field: model"}}`

**Invalid Model Field (400 Bad Request):**
- Condition: `model` field exists but is not a valid string
- Response: `{"type":"error","error":{"type":"invalid_request","message":<parse_error_details>}}`

**Request Too Large (413 Request Entity Too Large):**
- Condition: Request body exceeds 10 MiB
- Response: `{"type":"error","error":{"type":"invalid_request","message":"request body too large"}}`

**Upstream Unavailable (502 Bad Gateway):**
- Condition: HTTP client fails to connect to upstream
- Response: `{"type":"error","error":{"type":"proxy_error","message":"upstream unavailable: <error_details>"}}`

## State Transitions

| Upstream Status | Client Receives | Body Modified |
|-----------------|-----------------|---------------|
| 2xx Success | 2xx Success | No |
| 4xx Client Error | 4xx Same code | No |
| 5xx Server Error | 5xx Same code | No |
| Connection Failed | 502 Bad Gateway | Yes (proxy error) |

## Notable Behaviors

1. **No Error Transformation**: Upstream error bodies pass through completely unmodified. The proxy does not parse, reformat, or sanitize error content from upstream services.

2. **Status Code Preservation**: The exact HTTP status code from upstream is used. A 401 from Anthropic becomes 401 to client; a 500 from local server becomes 500 to client.

3. **Header Preservation**: All upstream response headers are copied, including error-specific headers like `X-Request-Id` for debugging.

4. **Error Type Distinction**: Proxy-generated errors use `type:"error"` at the root level with nested `error.type` and `error.message`. This matches Anthropic's error format for consistency.

## Error Handling Rationale

The proxy treats upstream errors as opaque data. This design:
- Preserves upstream error semantics exactly
- Allows clients to see authentic error information from the actual service
- Avoids introducing new failure points in error handling code
- Maintains compatibility with any upstream service implementing the API

## Examples

### Anthropic API Error Passthrough

**Upstream Response:**
```
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "messages: unexpected length. got 1, want at least 1"
  }
}
```

**Client Receives:** Identical response unchanged.

### Local Server Error Passthrough

**Upstream Response:**
```
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error": "internal server error"}
```

**Client Receives:** Identical response unchanged.

### Proxy-Generated Error

**Condition:** Request body is not valid JSON

**Client Receives:**
```
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "type": "error",
  "error": {
    "type": "invalid_request",
    "message": "invalid character 'x' looking for beginning of value"
  }
}
```
