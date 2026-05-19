# 14. Header Filtering and Forwarding

## Topic Statement

Filter and forward HTTP headers based on destination backend, applying different rules for local inference servers versus Anthropic API.

## Scope

**In-scope:**
- Header filtering for requests to Anthropic API
- Header filtering for requests to local inference servers
- Header preservation in responses

**Boundaries:**
- Input: Incoming HTTP request headers
- Output: Filtered headers for upstream request
- Does not include request body transformation (covered by thinking block stripping spec)

## Data Contracts

### Allowed Headers for Anthropic Forwarding (Do)

| Header | Forwarded | Purpose |
|--------|-----------|---------|
| `Content-Type` | Yes | Request/response content type |
| `X-Api-Key` | Yes | Anthropic API authentication |
| `Authorization` | Yes | Alternative authentication |
| `Anthropic-Version` | Yes | API version specification |
| `Anthropic-Beta` | Yes | Beta feature flags |

### Allowed Headers for Local Forwarding (DoLocal)

| Header | Forwarded | Purpose |
|--------|-----------|---------|
| `Content-Type` | Yes | Request/response content type |
| `Anthropic-Version` | Yes | API version specification |
| `Anthropic-Beta` | Yes | Beta feature flags (may be filtered) |

### Excluded Headers for Local Forwarding

| Header | Excluded | Reason |
|--------|----------|--------|
| `Authorization` | Yes | Prevents sending API keys to untrusted local servers |
| `X-Api-Key` | Yes | Prevents sending API keys to untrusted local servers |

### Response Header Handling

| Header | Standard Forward (Do) | Local Forward (DoLocal) |
|--------|----------------------|-------------------------|
| All upstream headers | Copied | Copied |
| `Content-Length` | Copied | Skipped (body may change) |
| `Transfer-Encoding` | Copied | Skipped (body may change) |

## Behaviors

### Standard Forward Header Processing (Do)

For requests to Anthropic API:

1. Initialize empty header map for outgoing request
2. For each header in allowed list:
   - Get header value from incoming request
   - If value is non-empty, set in outgoing request headers
3. Execute request with filtered headers
4. Copy all response headers from upstream to client

### Local Forward Header Processing (DoLocal)

For requests to local inference servers:

1. Initialize empty header map for outgoing request
2. For each header in allowed local list:
   - Get header value from incoming request
   - If value is non-empty, set in outgoing request headers
3. Execute request with filtered headers
4. Copy response headers from upstream, excluding:
   - `Content-Length` (body size may change after thinking block removal)
   - `Transfer-Encoding` (body size may change after thinking block removal)
5. Set appropriate `Content-Length` after body modification

### Anthropic-Beta Header Filtering

When forwarding to local servers, the `Anthropic-Beta` header undergoes additional filtering:

1. Split header value by comma
2. Trim whitespace from each token
3. Exclude tokens containing "thinking" substring
4. Join remaining tokens with comma
5. Set filtered header value

Example:
- Input: `interleaved-thinking-2025-05-14,prompt-caching-2024-07-31`
- Output: `prompt-caching-2024-07-31`

## State Transitions

| Input Header | Destination | Action |
|--------------|-------------|--------|
| `Content-Type` | Anthropic | Forward |
| `Content-Type` | Local | Forward |
| `X-Api-Key` | Anthropic | Forward |
| `X-Api-Key` | Local | Drop |
| `Authorization` | Anthropic | Forward |
| `Authorization` | Local | Drop |
| `Anthropic-Version` | Anthropic | Forward |
| `Anthropic-Version` | Local | Forward |
| `Anthropic-Beta` | Anthropic | Forward (unchanged) |
| `Anthropic-Beta` | Local | Forward (filter thinking tokens) |
| Other headers | Any | Drop |

## Notable Behaviors

1. **Security by default**: Authorization and API key headers are never forwarded to local inference servers, preventing accidental exposure of credentials

2. **Minimal header set**: Only explicitly allowed headers are forwarded, reducing attack surface and preventing header injection attacks

3. **Beta header sanitization**: Thinking-related beta tokens are removed when forwarding to local servers to prevent compatibility issues

4. **Response header preservation**: All upstream response headers are copied except where body modification requires skipping `Content-Length` and `Transfer-Encoding`

5. **Case-sensitive header matching**: Header names are matched exactly as specified in the allowed lists

## Error Handling

| Condition | Behavior |
|-----------|----------|
| No matching headers | Request forwarded with no custom headers |
| Empty header value | Header not included in outgoing request |
| Invalid beta header format | Pass through unchanged (no parsing error) |

## Rationale

Header filtering serves multiple purposes:

1. **Security**: Prevents API keys from being sent to potentially untrusted local inference servers. Local servers typically don't require authentication, and forwarding credentials could expose them unnecessarily.

2. **Compatibility**: The `Anthropic-Beta` header may contain tokens that are incompatible with local inference servers. Filtering thinking-related tokens prevents the local server from attempting to use features it doesn't support.

3. **Predictability**: By explicitly listing allowed headers, the proxy ensures consistent behavior regardless of what headers the client sends. Unexpected headers are ignored rather than forwarded.

4. **Response integrity**: When the body is modified (thinking block removal), the `Content-Length` and `Transfer-Encoding` headers from the upstream would be incorrect. Skipping these headers and recalculating `Content-Length` ensures response integrity.

## Examples

### Example 1: Request to Anthropic

Incoming request headers:
```
Content-Type: application/json
X-Api-Key: sk-ant-xxx
Authorization: Bearer token
Anthropic-Version: 2023-06-01
Anthropic-Beta: thinking-2025-05-14
Custom-Header: value
```

Forwarded to Anthropic:
```
Content-Type: application/json
X-Api-Key: sk-ant-xxx
Authorization: Bearer token
Anthropic-Version: 2023-06-01
Anthropic-Beta: thinking-2025-05-14
```

### Example 2: Request to Local Server

Incoming request headers:
```
Content-Type: application/json
X-Api-Key: sk-ant-xxx
Authorization: Bearer token
Anthropic-Version: 2023-06-01
Anthropic-Beta: thinking-2025-05-14,prompt-caching-2024-07-31
Custom-Header: value
```

Forwarded to local server:
```
Content-Type: application/json
Anthropic-Version: 2023-06-01
Anthropic-Beta: prompt-caching-2024-07-31
```
