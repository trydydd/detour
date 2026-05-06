# 08. Error Handling and Response Formats

## Topic Statement

Generate consistent error responses for invalid requests and upstream failures.

## Scope

**In-scope:**
- HTTP error status codes
- Error response body format
- Error type classification

**Boundaries:**
- Input: error condition
- Output: HTTP response with error body

## Data Contracts

### Error Response Format

All error responses use JSON format:

```json
{
  "type": "error",
  "error": {
    "type": "<error_type>",
    "message": "<human_readable_message>"
  }
}
```

### Error Types

| Error Type | Meaning |
|------------|---------|
| `invalid_request` | Client sent malformed or invalid request |
| `proxy_error` | Internal proxy failure or upstream unavailable |

### HTTP Status Code Mapping

| Condition | Status Code | Error Type |
|-----------|-------------|------------|
| Invalid JSON body | 400 | invalid_request |
| Missing required field | 400 | invalid_request |
| Request body too large | 413 | invalid_request |
| Upstream connection failure | 502 | proxy_error |
| Internal processing error | 500 | proxy_error |

## Behaviors

### Error Response Construction

1. Set `Content-Type` header to `application/json`
2. Write HTTP status code
3. Encode error object as JSON:
   - `type`: Always `"error"` at root level
   - `error.type`: Classification of error
   - `error.message`: Human-readable description

### Validation Error Messages

| Condition | Message |
|-----------|---------|
| Body read failure | "could not read request body" |
| Body exceeds limit | "request body too large" |
| Invalid JSON | JSON parse error details |
| Missing model field | "missing required field: model" |

### Upstream Error Messages

| Condition | Message Pattern |
|-----------|-----------------|
| Request creation failure | "<details>" (from underlying error) |
| Upstream unavailable | "upstream unavailable: <details>" |

## State Transitions

| Error Condition | Status | Type | Message |
|-----------------|--------|------|---------|
| Invalid JSON | 400 | invalid_request | Parse error |
| Missing model | 400 | invalid_request | "missing required field: model" |
| Body too large | 413 | invalid_request | "request body too large" |
| Upstream down | 502 | proxy_error | "upstream unavailable: <details>" |
| Internal error | 500 | proxy_error | Error details |

## Notable Behaviors

1. Consistent JSON structure across all error responses
2. Error type provides machine-readable classification
3. Message provides human-readable context
4. Upstream status codes (4xx, 5xx) forwarded unchanged to client
