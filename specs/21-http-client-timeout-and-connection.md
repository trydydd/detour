# HTTP Client Timeout and Connection Management

## Topic Statement

This spec defines the HTTP client behavior for upstream connections including timeouts, connection reuse, and error handling.

## Scope

**In-scope:**
- HTTP client configuration for upstream requests
- Connection lifecycle and reuse
- Timeout behavior (none configured)

**Boundaries:**
- Does not cover proxy server listening (see spec 10)
- Does not cover graceful shutdown (see spec 17)
- Does not cover request/response transformation (see spec 20)

## Data Contracts

### HTTP Client Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| Client Instance | `http.DefaultClient` | Go's global default HTTP client |
| Transport | Default | `http.DefaultTransport` |
| Timeout | None configured | No explicit timeout set |
| Idle Timeout | 90 seconds | From `http.DefaultTransport.IdleConnTimeout` |
| Max Idle Conns | 100 | From `http.DefaultTransport.MaxIdleConns` |
| Max Idle Conns Per Host | 90 | From `http.DefaultTransport.MaxIdleConnsPerHost` |

### Connection Pool Behavior

| Metric | Value | Source |
|--------|-------|--------|
| Idle connection timeout | 90s | `http.DefaultTransport.IdleConnTimeout` |
| Maximum idle connections | 100 | `http.DefaultTransport.MaxIdleConns` |
| Maximum per-host idle | 90 | `http.DefaultTransport.MaxIdleConnsPerHost` |
| Response header timeout | None | Not configured |
| Dial timeout | No explicit limit | Depends on system |
| TLS handshake timeout | 10s | `http.DefaultTransport.TLSHandshakeTimeout` |

## Behaviors

### Connection Lifecycle

1. **New Request**: When `Do()` or `DoLocal()` is called, the HTTP client attempts to establish or reuse a connection to the upstream target.

2. **Connection Reuse**: Connections to the same upstream host are reused up to the maximum idle connection limit (90 per host).

3. **Idle Timeout**: Connections idle for more than 90 seconds are closed and removed from the pool.

4. **Connection Cleanup**: When the proxy shuts down, idle connections are closed as part of HTTP server shutdown.

### Timeout Behavior

**Important: No explicit request timeout is configured.**

- **Request Duration**: Requests can take unlimited time to complete
- **Read Timeout**: No limit on response body read duration
- **Write Timeout**: No limit on request body write duration
- **Dial Duration**: No explicit limit on connection establishment

### Error Conditions

**Connection Failure:**
- Condition: Cannot establish connection to upstream
- Cause: Network unreachable, host down, firewall blocking
- Response: HTTP 502 Bad Gateway with message "upstream unavailable: <error>"

**Timeout (System Default):**
- Condition: Connection takes longer than system defaults
- Cause: Network congestion, slow DNS, routing issues
- Response: Depends on underlying error, typically 502 Bad Gateway

**TLS Handshake Timeout:**
- Condition: TLS handshake exceeds 10 seconds
- Cause: Slow TLS server, network latency
- Response: Error from client.Do(), typically 502 Bad Gateway

## State Transitions

| Condition | Connection State | Client Response |
|-----------|------------------|-----------------|
| Successful connection | Reused or new | Forward response |
| Connection pool full | New connection created | Forward response |
| Idle connection expired | New connection required | Forward response |
| Connection refused | Failed | 502 Bad Gateway |
| Host unreachable | Failed | 502 Bad Gateway |
| TLS handshake timeout | Failed | 502 Bad Gateway |

## Notable Behaviors

1. **No Request Timeout**: The proxy does not configure any explicit timeout for upstream requests. Long-running requests will block until completion or connection failure.

2. **Default Transport**: Uses Go's `http.DefaultTransport` which provides:
   - Connection pooling for performance
   - Automatic retry behavior for certain errors
   - 10 second TLS handshake timeout
   - 90 second idle connection timeout

3. **Connection Reuse**: Multiple requests to the same upstream (Anthropic or local server) reuse TCP connections, reducing latency.

4. **No Request Cancellation**: If the client disconnects, the upstream request continues unless the context is explicitly cancelled.

5. **No Retry Logic**: The proxy does not implement application-level retry logic. Failed requests return immediately with 502.

## Rationale

The use of `http.DefaultClient` with no custom configuration:

1. **Simplicity**: Reduces code complexity by relying on Go's well-tested defaults.

2. **Connection Pooling**: Default transport provides efficient connection reuse without additional code.

3. **No Timeout Constraints**: Allows upstream services to respond at their own pace, important for:
   - Long-running inference tasks
   - Streaming responses of variable duration
   - Slow local inference servers

4. **Trade-offs**:
   - **Pros**: Simple, reliable, handles most cases well
   - **Cons**: No protection against extremely slow or hung upstream requests

## Examples

### Normal Request Flow

```
Client Request → Proxy → Connection to Upstream (reused if available)
                          ↓
                    Upstream Response
                          ↓
                    Proxy → Client
```

### Connection Pool Exhaustion

When 90 connections to the same host are idle:
- New requests still succeed
- Oldest idle connections are closed to make room
- Connection churn increases slightly

### Upstream Unavailable

```
Client Request → Proxy → Connection Refused
                          ↓
                    502 Bad Gateway
                    {"type":"error","error":{"type":"proxy_error","message":"upstream unavailable: connection refused"}}
```
