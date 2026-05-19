# Request and Response Transformation Pipeline Order

## Topic Statement

This spec defines the exact sequence of transformations applied to requests and responses as they flow through the detour proxy.

## Scope

**In-scope:**
- Complete ordered sequence of all request transformations
- Complete ordered sequence of all response transformations
- Differences between local and passthrough routes

**Boundaries:**
- Does not define individual transformations (referenced from other specs)
- Does not cover configuration loading (see spec 01)
- Does not cover error handling (see spec 08)

## Data Contracts

### Request Transformation Pipeline

| Step | Transformation | Applies To | Description |
|------|----------------|------------|-------------|
| 1 | Read request body | All | Read JSON body with 10 MiB limit |
| 2 | Extract model field | All | Parse `model` field from JSON for routing |
| 3 | Route decision | All | Determine local vs passthrough based on model name |
| 4a | Strip thinking field | Local only | Remove `thinking` key from JSON body |
| 4b | Pass through unchanged | Passthrough | No transformation |
| 5a | Filter Anthropic-Beta header | Local only | Remove thinking-related beta flags |
| 5b | Pass headers unchanged | Passthrough | No transformation |
| 6a | Use DoLocal() | Local only | Forward with thinking stripping from response |
| 6b | Use Do() | Passthrough | Forward without modification |

### Response Transformation Pipeline (Local Route Only)

| Step | Transformation | Description |
|------|----------------|-------------|
| 1 | Read upstream response | Get status code and headers |
| 2 | Skip Content-Length/Transfer-Encoding | Do not copy these headers |
| 3 | Copy other headers | All remaining headers passed through |
| 4a | Streaming path | Apply `copyStreamingStripThinking()` |
| 4b | Non-streaming path | Apply `stripThinkingFromResponseBody()` |
| 5 | Write response | Send to client with calculated Content-Length |

### Response Transformation Pipeline (Passthrough Route Only)

| Step | Transformation | Description |
|------|----------------|-------------|
| 1 | Read upstream response | Get status code and headers |
| 2 | Copy all headers | Including Content-Length |
| 3 | Write status code | Pass through unchanged |
| 4a | Streaming path | Apply `copyStreaming()` - no modification |
| 4b | Non-streaming path | Copy body unchanged |
| 5 | Write response | Send to client |

## Behaviors

### Request Processing Order

1. **Body Reading**: HTTP request body is read into memory with a 10 MiB limit. If body exceeds limit, return 413 error immediately.

2. **Model Extraction**: JSON is parsed to extract the `model` field value. If parsing fails, return 400 error. If `model` is missing or empty, return 400 error.

3. **Routing Decision**: The extracted model name is compared against the configured `ModelName`. Exact string match → local route. No match → passthrough route.

4. **Request Transformation (Local Only)**:
   - The `thinking` field is removed from the JSON body by deleting the key and re-marshaling
   - The `Anthropic-Beta` header is parsed, and any comma-separated value containing "thinking" is removed
   - These transformations happen before forwarding to local upstream

5. **Request Forwarding**:
   - Local route: `DoLocal()` forwards to `LocalUpstreamURL/v1/messages`
   - Passthrough route: `Do()` forwards to `AnthropicUpstreamURL/v1/messages`

### Response Processing Order (Local Route)

1. **Header Filtering**: Response headers are copied except `Content-Length` and `Transfer-Encoding` which are skipped because the body size may change.

2. **Body Transformation**:
   - **Non-streaming**: Complete body is read, parsed as JSON, content array is filtered to remove thinking blocks, then re-marshaled
   - **Streaming**: Each SSE event is processed; thinking block events are dropped; message_start events are patched to add missing type/role fields

3. **Response Writing**: Transformed body is written with calculated Content-Length header.

### Response Processing Order (Passthrough Route)

1. **Header Copying**: All response headers from upstream are copied including Content-Length and Transfer-Encoding.

2. **Body Forwarding**:
   - **Non-streaming**: Body is copied byte-for-byte without modification
   - **Streaming**: Each line is copied with flushing after each chunk

3. **Response Writing**: Original status code and all headers are preserved.

## State Transitions

### Request Flow

| Input | Routing | Request Body After Transform | Headers After Transform |
|-------|---------|------------------------------|-------------------------|
| model=red | Local | thinking field removed | Beta header filtered |
| model=claude-opus-4-7 | Passthrough | Unchanged | Unchanged |

### Response Flow (Local Route)

| Upstream Response | Headers Copied | Body After Transform |
|-------------------|----------------|----------------------|
| Streaming with thinking | All except CL/TE | Thinking events dropped |
| Non-streaming with thinking | All except CL/TE | Thinking blocks removed |
| Streaming without thinking | All except CL/TE | Unchanged |
| Non-streaming without thinking | All except CL/TE | Unchanged |

## Notable Behaviors

1. **Thinking Removal Happens Twice**: For local routes, thinking is stripped from both the request (before sending to local server) and the response (before sending to client). This ensures no invalid thinking signatures propagate.

2. **Header Order Matters**: The `Anthropic-Beta` header is filtered after routing decision but before forwarding. This prevents thinking-related beta flags from reaching local servers.

3. **Content-Length Recalculation**: For local routes, Content-Length cannot be copied from upstream because the transformed body may have different size. It is calculated after transformation.

4. **Streaming vs Non-Streaming Divergence**: The transformation pipeline branches based on whether the upstream response is streaming. Non-streaming uses JSON parsing; streaming uses line-by-line SSE parsing.

5. **Message Start Patching Only for Local**: The `patchMessageStart()` function that adds `type:"message"` and `role:"assistant"` only runs for local routes via `copyStreamingStripThinking()`. Passthrough routes receive raw upstream data.

## Rationale

The transformation order is designed to:

1. **Preserve Security**: Thinking signatures are validated by Anthropic. Local servers cannot produce valid signatures, so thinking must be removed to prevent signature validation failures downstream.

2. **Maintain Compatibility**: Some local inference servers (vLLM) omit required fields. The message_start patching ensures compatibility with clients that expect complete messages.

3. **Minimize Transformation Overhead**: Passthrough routes have zero transformation overhead, preserving upstream behavior exactly.

4. **Ensure Consistency**: The same transformation logic applies regardless of how the request arrived, ensuring predictable behavior.

## Examples

### Local Route Request Transformation

**Original Request:**
```json
{
  "model": "red",
  "messages": [{"role": "user", "content": "Hello"}],
  "thinking": "hidden thought process",
  "max_tokens": 1000
}
```

**After Transformation (forwarded to local server):**
```json
{
  "model": "red",
  "messages": [{"role": "user", "content": "Hello"}],
  "max_tokens": 1000
}
```

**Header Transformation:**
- Original `Anthropic-Beta: thinking-1, cache-control-1`
- Transformed `Anthropic-Beta: cache-control-1`

### Local Route Response Transformation (Streaming)

**Upstream SSE Events:**
```
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"thinking..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"response"}}
```

**Client Receives (thinking events dropped):**
```
event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"response"}}
```

### Passthrough Route - No Transformation

**Original Request:**
```json
{
  "model": "claude-opus-4-7",
  "messages": [{"role": "user", "content": "Hello"}],
  "thinking": "some thought",
  "max_tokens": 1000
}
```

**Forwarded to Anthropic:** Identical, no transformation.
