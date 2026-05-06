# 04. Request Forwarding to Upstream Services

## Topic Statement

Forward HTTP requests to upstream services, handling both streaming and non-streaming responses, with appropriate header management.

## Scope

**In-scope:**
- Outbound HTTP request creation
- Header filtering and forwarding
- Response copying (streaming and non-streaming)
- Error handling for upstream failures

**Boundaries:**
- Input: client request and target upstream URL
- Output: response to client
- Does not include request transformation (handled by proxy layer)

## Data Contracts

### Allowed Headers for Anthropic Forwarding

| Header | Purpose |
|--------|---------|
| `Content-Type` | Request/response content type |
| `X-Api-Key` | Anthropic API authentication |
| `Authorization` | Alternative authentication |
| `Anthropic-Version` | API version specification |
| `Anthropic-Beta` | Beta feature flags |

### Allowed Headers for Local Forwarding

| Header | Purpose |
|--------|---------|
| `Content-Type` | Request/response content type |
| `Anthropic-Version` | API version specification |
| `Anthropic-Beta` | Beta feature flags (filtered) |

**Excluded from local forwarding:**
- `Authorization` - not forwarded to local inference servers
- `X-Api-Key` - not forwarded to local inference servers

## Behaviors

### Standard Forward (Do)

Sequence for forwarding to Anthropic:

1. Create new HTTP request with same method, target URL, and body
2. Copy allowed headers from original request
3. Execute request using default HTTP client
4. On error: return 502 Bad Gateway with error details
5. Copy all response headers from upstream to client
6. Write upstream status code to client
7. Copy response body:
   - If streaming: use streaming copy with flush after each chunk
   - If non-streaming: copy entire body at once

### Local Forward (DoLocal)

Sequence for forwarding to local inference server:

1. Create new HTTP request with same method, target URL, and body
2. Copy only allowed local headers from original request
3. Execute request using default HTTP client
4. On error: return 502 Bad Gateway with error details
5. Copy response headers excluding `Content-Length` and `Transfer-Encoding`
6. Process response body:
   - If streaming: use streaming strip-thinking copy with flush after each chunk
   - If non-streaming: read entire body, strip thinking blocks, set Content-Length
7. Write upstream status code to client
8. Write processed body to client

### Streaming Copy Behavior

For responses with `Content-Type: text/event-stream`:

1. Read response body in 4096-byte chunks
2. Write each chunk to client
3. Flush after each write to ensure immediate delivery
4. Continue until read returns error (end of stream)

### Error Responses

| Condition | HTTP Status | Error Type | Message Pattern |
|-----------|-------------|------------|-----------------|
| Request creation failure | 500 | proxy_error | Request creation error details |
| Upstream connection failure | 502 | proxy_error | "upstream unavailable: <details>" |

## State Transitions

| Forward Type | Headers Forwarded | Body Processing |
|--------------|-------------------|-----------------|
| Standard (Do) | Allowed headers list | Direct copy |
| Local (DoLocal) | Allowed local headers | Strip thinking blocks |

## Notable Behaviors

1. Authorization header excluded from local forwarding prevents sending API keys to untrusted local servers
2. Content-Length and Transfer-Encoding headers skipped for local forwarding because body size may change after thinking block removal
3. Streaming responses processed chunk-by-chunk with immediate flush to maintain real-time behavior
4. All upstream status codes forwarded unchanged (including 4xx errors)
