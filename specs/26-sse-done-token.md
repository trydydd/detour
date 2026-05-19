# 26. SSE [DONE] Token Handling

## Topic Statement

Handle the `[DONE]` token in Server-Sent Events (SSE) streaming responses as a stream termination indicator.

## Scope

**In-scope:**
- Detection of `[DONE]` token in SSE data
- Treatment of `[DONE]` as a non-droppable event
- Stream continuation after `[DONE]` token

**Boundaries:**
- Input: SSE stream from upstream servers
- Output: Forwarded stream with `[DONE]` preserved
- Only applies to streaming responses

## Data Contracts

### [DONE] Token Format

```
data: [DONE]
```

The `[DONE]` token appears as the data payload of an SSE event. It may appear:
- As a standalone event (typically the final event)
- With or without an `event:` type line preceding it

## Behaviors

### [DONE] Detection

During SSE stream processing:

1. Parse each event line looking for `data:` prefix
2. Extract the data payload
3. Check if data equals `[DONE]` (exact string match)
4. If `[DONE]` detected:
   - Do NOT drop the event
   - Forward event unchanged to client
   - Continue processing any remaining events

### Event Filtering Interaction

The `[DONE]` token is explicitly excluded from thinking block filtering:

```
if data == "[DONE]" {
    return false  // Do not drop
}
```

This ensures `[DONE]` is always forwarded regardless of thinking block state.

### Stream Termination

The `[DONE]` token signals end of streaming response but:
- The proxy does not close the stream upon seeing `[DONE]`
- The proxy continues forwarding any subsequent events
- The actual stream closure is handled by the underlying HTTP connection

## State Transitions

| Event Data | Thinking Index State | Action |
|------------|---------------------|--------|
| `[DONE]` | Any | Forward unchanged |
| `[DONE]` | In thinking block | Forward (exempt from filtering) |
| Other | Any | Apply normal filtering rules |

## Notable Behaviors

1. **Exempt from thinking filtering**: `[DONE]` is explicitly checked before thinking block index lookup, ensuring it is never dropped even if a thinking block is active

2. **No special processing**: Beyond exemption from filtering, `[DONE]` receives no special treatment - it is forwarded as-is

3. **OpenAI compatibility**: The `[DONE]` token is an OpenAI streaming convention that some local inference servers adopt for Anthropic-compatible APIs

4. **Does not terminate proxy stream**: The proxy continues processing after `[DONE]`; stream termination is handled by the underlying HTTP connection closure

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Malformed `[DONE]` (e.g., `[DON]`) | Treated as regular data, forwarded unchanged |
| `[DONE]` in thinking block | Still forwarded (exempt from filtering) |
| Multiple `[DONE]` tokens | Each forwarded independently |

## Rationale

The `[DONE]` token handling serves several purposes:

1. **Client compatibility**: Some clients expect `[DONE]` as a stream termination signal and may hang or error without it

2. **Standard convention**: `[DONE]` is a widely-used convention in LLM streaming APIs (OpenAI, vLLM, etc.)

3. **Transparency**: Forwarding `[DONE]` unchanged preserves the upstream server's intended stream structure

4. **No interference**: By explicitly exempting `[DONE]` from thinking block filtering, the proxy ensures this control token is never accidentally dropped

## Examples

### Standard Stream with [DONE]

**Upstream Sends:**
```
event: message_start
data: {"type":"message_start",...}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}

event: end
data: [DONE]
```

**Client Receives:** Identical - all events including `[DONE]` forwarded

### [DONE] During Thinking Block

**Upstream Sends:**
```
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"thinking..."}}

event: end
data: [DONE]
```

**Client Receives:**
```
event: end
data: [DONE]
```

Thinking events dropped, but `[DONE]` forwarded (exempt from filtering)

## Implementation Notes

The `[DONE]` check occurs in `shouldDropSSEEvent()` function, which is called for every SSE event during streaming. The check is performed early in the function, before any thinking block index checks, ensuring `[DONE]` is never dropped.
