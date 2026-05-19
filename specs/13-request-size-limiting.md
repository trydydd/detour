# 13. Request Size Limiting and Validation

## Topic Statement

Enforce maximum request body size limits to prevent resource exhaustion and reject oversized requests with appropriate error responses.

## Scope

**In-scope:**
- Request body size validation
- Oversized request rejection
- Error response generation for size violations

**Boundaries:**
- Input: HTTP request bodies
- Output: Either processed request or size limit error response
- Applies only to `/v1/messages` endpoint (other endpoints have no size limit)

## Data Contracts

### Size Limit Configuration

| Parameter | Value | Purpose |
|-----------|-------|---------|
| Maximum request size | 10 MiB (10,485,760 bytes) | Prevent resource exhaustion |

### Read Buffer Behavior

- Initial read uses `io.LimitReader` with limit set to `maxRequestBytes + 1`
- Reading one byte beyond the limit allows detection of overflow
- If read returns more than `maxRequestBytes`, request is rejected

## Behaviors

### Request Size Validation Sequence

For `/v1/messages` requests:

1. Initialize read with limit of `maxRequestBytes + 1` (10,485,761 bytes)
2. Read request body into buffer
3. If read returns error:
   - Return 400 Bad Request with error type `invalid_request`
   - Message: "could not read request body"
4. If read completes successfully:
   - Check if actual body length exceeds `maxRequestBytes`
   - If exceeded: Return 413 Request Entity Too Large
   - If within limit: Proceed with request processing

### Error Response for Oversized Requests

HTTP Status: `413 Request Entity Too Large`

Error response body:
```json
{
  "type": "error",
  "error": {
    "type": "invalid_request",
    "message": "request body too large"
  }
}
```

### Error Response for Read Failures

HTTP Status: `400 Bad Request`

Error response body:
```json
{
  "type": "error",
  "error": {
    "type": "invalid_request",
    "message": "could not read request body"
  }
}
```

## State Transitions

| Input Condition | HTTP Status | Error Type | Processing |
|-----------------|-------------|------------|------------|
| Body read error | 400 | invalid_request | Aborted |
| Body > 10 MiB | 413 | invalid_request | Aborted |
| Body <= 10 MiB | Continue | — | Normal processing |

## Notable Behaviors

1. **One-byte overflow detection**: Using `maxRequestBytes + 1` as the limit allows the code to detect when a body exceeds the limit without fully reading it

2. **Early rejection**: Oversized requests are rejected before any JSON parsing or model extraction occurs

3. **Consistent error format**: Size limit errors use the same JSON error format as other validation errors

4. **Only applies to messages endpoint**: The `/v1/models` and `/health` endpoints do not have size limits

5. **No streaming size limit**: Streaming responses from upstream are not size-limited on the response side

## Rationale

Request size limiting serves several purposes:

1. **Resource protection**: Prevents memory exhaustion from arbitrarily large requests
2. **Denial of service mitigation**: Limits the impact of malicious or accidental oversized requests
3. **Predictable behavior**: Ensures the proxy can handle requests within known bounds
4. **Early failure**: Rejects oversized requests quickly without wasting processing cycles

The 10 MiB limit was chosen as a practical upper bound that accommodates typical API usage while preventing abuse. This limit is generous enough for:
- Long conversation histories
- Large context windows
- Multiple messages in a single request

But restrictive enough to prevent:
- Memory exhaustion attacks
- Accidental submission of extremely large files
- Resource starvation on the server

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Read returns error | Return 400 with "could not read request body" |
| Body length > maxRequestBytes | Return 413 with "request body too large" |
| Body length <= maxRequestBytes | Proceed with normal processing |

## Testing Scenarios

### Valid Request
- Input: 5 MiB JSON body with valid structure
- Expected: Request processed normally

### Oversized Request
- Input: 11 MiB JSON body
- Expected: 413 error with "request body too large" message

### Edge Case
- Input: Exactly 10 MiB body
- Expected: Request processed normally (at limit but not over)

### Edge Case
- Input: 10 MiB + 1 byte body
- Expected: 413 error (one byte over limit)
