# SSE Event Streaming Protocol

## Topic Statement

This spec defines the Server-Sent Events (SSE) streaming format used for real-time response delivery through the detour proxy.

## Scope

**In-scope:**
- SSE event format and structure
- Streaming event sequence for Anthropic Messages API
- Event filtering and transformation rules

**Boundaries:**
- Does not cover non-streaming responses (see spec 03)
- Does not cover thinking block removal logic (see spec 05)
- Does not cover message_start patching details (see spec 11)

## Data Contracts

### SSE Event Format

Each SSE event consists of one or more lines terminated by double newline:

```
event: <event_type>
data: <json_payload>

```

| Component | Description |
|-----------|-------------|
| `event:` | Event type identifier |
| `data:` | JSON-encoded event payload |
| Blank line | Event delimiter |

### Event Types

| Event Type | Direction | Description |
|------------|-----------|-------------|
| `message_start` | Upstream→Client | Beginning of message response |
| `content_block_start` | Upstream→Client | Beginning of content block |
| `content_block_delta` | Upstream→Client | Content increment (text/thinking) |
| `content_block_stop` | Upstream→Client | End of content block |
| `message_delta` | Upstream→Client | Message-level updates |
| `message_stop` | Upstream→Client | End of message |
| `[DONE]` | Upstream→Client | Stream termination indicator (OpenAI compatibility) |

### Event Payload Structures

**message_start:**
```json
{
  "type": "message_start",
  "message": {
    "id": "msg_...",
    "type": "message",
    "role": "assistant",
    "content": [],
    "model": "<model_name>",
    "stop_reason": null,
    "stop_sequence": null,
    "usage": {"input_tokens": <int>, "output_tokens": <int>}
  }
}
```

**content_block_start:**
```json
{
  "type": "content_block_start",
  "index": <int>,
  "content_block": {"type": "text"|"thinking", ...}
}
```

**content_block_delta:**
```json
{
  "type": "content_block_delta",
  "index": <int>,
  "delta": {"type": "text_delta"|"thinking_delta", "text": "<string>"}
}
```

**content_block_stop:**
```json
{
  "type": "content_block_stop",
  "index": <int>
}
```

**message_delta:**
```json
{
  "type": "message_delta",
  "delta": {"stop_reason": "<reason>", "stop_sequence": <string|null>},
  "usage": {"output_tokens": <int>}
}
```

**message_stop:**
```json
{
  "type": "message_stop"
}
```

## Behaviors

### Streaming Sequence (Normal Flow)

1. **message_start**: Initial event containing message metadata
2. **content_block_start**: Start of first content block (index 0)
3. **content_block_delta**: One or more deltas containing incremental text
4. **content_block_stop**: End of content block
5. **message_delta**: Final message updates and token usage
6. **message_stop**: Stream termination

### Streaming Sequence (With Thinking)

1. **message_start**: Initial message metadata
2. **content_block_start** (index 0, type=thinking): Start of thinking block
3. **content_block_delta** (index 0, thinking_delta): Thinking content
4. **content_block_stop** (index 0): End of thinking block
5. **content_block_start** (index 1, type=text): Start of text block
6. **content_block_delta** (index 1, text_delta): Text content
7. **content_block_stop** (index 1): End of text block
8. **message_delta**: Final updates
9. **message_stop**: Stream termination

### Event Filtering (Local Routes Only)

For local upstream routes, the proxy filters events as follows:

1. **Track Thinking Indices**: When `content_block_start` has `type="thinking"`, record that index as a thinking block.

2. **Drop Thinking Events**: For all events with a tracked thinking index:
   - `content_block_start` (thinking type)
   - `content_block_delta` (thinking index)
   - `content_block_stop` (thinking index)

3. **Pass Through Other Events**: All non-thinking events pass unchanged.

### Message Start Patching (Local Routes Only)

For local upstream routes, `message_start` events may be patched:

1. **Check for Required Fields**: Examine the inner `message` object for `type` and `role` fields.

2. **Inject Missing Fields**:
   - If `type` is missing: Add `type: "message"`
   - If `role` is missing: Add `role: "assistant"`

3. **Re-encode**: Marshal the patched object back to JSON.

4. **Pass Through**: If both fields already exist, pass the event unchanged.

### Scanner Buffer Configuration

- **Initial Buffer Size**: 4096 bytes
- **Maximum Buffer Size**: 64 KiB (65536 bytes)
- **Purpose**: Handle long SSE events without buffer overflow

## State Transitions

### Thinking Index Tracking

| Event | Action | thinkingIdx State |
|-------|--------|-------------------|
| content_block_start (thinking) | Record index | {0: true} |
| content_block_delta (index 0) | Drop event | {0: true} |
| content_block_stop (index 0) | Drop event | {0: true} |
| content_block_start (text) | Pass event | {0: true} |
| content_block_delta (index 1) | Pass event | {0: true} |

### Event Filtering Decision

| Event Type | Index | Is Thinking? | Action |
|------------|-------|--------------|--------|
| content_block_start | 0 | Yes | Drop + track |
| content_block_delta | 0 | Yes | Drop |
| content_block_stop | 0 | Yes | Drop |
| content_block_start | 1 | No | Pass |
| content_block_delta | 1 | No | Pass |
| message_start | N/A | N/A | Pass (possibly patch) |
| message_delta | N/A | N/A | Pass |
| message_stop | N/A | N/A | Pass |
| [DONE] | N/A | N/A | Pass (exempt from thinking filtering) |

## Notable Behaviors

1. **Index-Based Tracking**: Thinking blocks are identified by their index in the content block sequence, not by event type alone.

2. **Stateful Filtering**: The `thinkingIdx` map maintains state across events to track which indices contain thinking content.

3. **Patching Only on Missing Fields**: message_start patching only adds fields that are missing; it never overwrites existing values.

4. **Flushing After Each Event**: Each SSE event is flushed immediately to the client for real-time delivery.

5. **Malformed Stream Handling**: If the stream ends without a blank line after the last event, the remaining event is still processed.

6. **No Event Reordering**: Events are processed and forwarded in the exact order received from upstream.

7. **[DONE] token handling**: The `[DONE]` token is always forwarded regardless of thinking block state. It is explicitly exempt from thinking block filtering to maintain compatibility with clients that expect this stream termination signal.

## Rationale

The streaming design:

1. **Real-time Delivery**: Flushing after each event provides immediate feedback to clients.

2. **Thinking Transparency**: Filtering thinking blocks for local routes prevents invalid signatures from propagating.

3. **Compatibility**: Message start patching ensures local servers produce output compatible with clients expecting full Anthropic spec.

4. **Memory Efficiency**: Line-by-line scanning with bounded buffers handles long streams without excessive memory use.

## Examples

### Complete Stream (No Thinking)

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"red","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
```

### Stream with Thinking (Filtered for Local Route)

**Upstream Sends:**
```
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"Let me think..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}
```

**Client Receives (thinking events dropped):**
```
event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}
```

### Message Start Patching

**Upstream Sends (missing type and role):**
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","content":[],"model":"red","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}
```

**Client Receives (fields injected):**
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","content":[],"model":"red","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}
```
