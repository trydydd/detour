# 12. Streaming Response Handling

## Topic Statement

Parse and process Server-Sent Events (SSE) streaming responses from upstream servers with proper buffering, event grouping, and filtering.

## Scope

**In-scope:**
- SSE stream parsing with buffered scanner
- Event grouping by blank line separators
- Event filtering based on content block type
- Streaming response forwarding with immediate flush
- Trailing event handling for malformed streams

**Boundaries:**
- Input: raw HTTP response body from upstream server
- Output: filtered SSE stream to client
- Only applies to responses with `Content-Type: text/event-stream`

## Data Contracts

### SSE Event Format

Standard SSE event structure:
```
event: <event_type>
data: <json_data>

```

Events are separated by blank lines. Each event contains:
- `event` line: specifies event type
- `data` line: contains JSON payload
- Blank line: marks end of event

### Supported Event Types

| Event Type | Purpose |
|------------|---------|
| `message_start` | Initial message metadata |
| `content_block_start` | Start of content block |
| `content_block_delta` | Content update (text, thinking, etc.) |
| `content_block_stop` | End of content block |
| `message_delta` | Final message metadata |
| `message_stop` | End of stream |
| `content_block` | Generic content block (deprecated) |

### Scanner Buffer Configuration

- Initial buffer size: 4096 bytes
- Maximum buffer size: 64 KiB (65536 bytes)
- Buffer grows as needed up to maximum

## Behaviors

### Stream Detection

1. Check response `Content-Type` header
2. If header starts with `text/event-stream`, use streaming path
3. Otherwise, use non-streaming path

### Streaming Copy (Standard)

For standard upstream forwarding (Do):

1. Create 4096-byte buffer
2. Read from upstream body in chunks
3. Write each chunk to client
4. Flush after each write
5. Continue until read returns error

### Streaming Copy with Thinking Filter (DoLocal)

For local upstream forwarding with thinking block filtering:

1. Initialize empty thinking block index tracker (map of int to bool)
2. Create buffered scanner with 64 KiB maximum buffer
3. Initialize event accumulator (slice of strings)
4. Read stream line by line:
   - If line is empty (blank line):
     - Process accumulated event if not empty
     - Check if event should be dropped via shouldDropSSEEvent
     - If not dropped: write each line of patched event, write blank line, flush
     - Reset event accumulator
   - If line is not empty:
     - Append line to event accumulator
5. After loop ends (stream complete):
   - If event accumulator has remaining lines (malformed trailing event):
     - Apply patching to each line
     - Write lines without requiring trailing blank line
     - Flush

### Event Processing Logic

For each complete SSE event:

1. Parse event data as JSON
2. Extract `type` field from event
3. Extract `index` field if present (content block index)
4. Extract `content_block.type` field if present (for content_block_start events)

### Thinking Block Detection

A content block is identified as thinking when:

1. Event type is `content_block_start` AND
2. `content_block.type` equals `"thinking"`

When detected:
- Record the block index in thinking tracker
- Drop this event (do not forward)

### Event Filtering Rules

| Event Type | Filter Condition | Action |
|------------|------------------|--------|
| `content_block_start` | `content_block.type == "thinking"` | Drop, record index |
| `content_block_delta` | Index in thinking tracker | Drop |
| `content_block_stop` | Index in thinking tracker | Drop |
| `message_start` | Never | Forward (apply patching) |
| `content_block_start` | Not thinking | Forward |
| `content_block_delta` | Index not in thinking tracker | Forward |
| `content_block_stop` | Index not in thinking tracker | Forward |
| `message_delta` | Never | Forward |
| `message_stop` | Never | Forward |
| `[DONE]` | Never | Forward (exempt from thinking filtering) |

### Trailing Event Handling

If the stream ends without a final blank line (malformed SSE):

1. Any remaining lines in event accumulator are treated as a complete event
2. Process and forward the event without requiring blank line terminator
3. Apply normal filtering rules
4. Flush after writing

## State Transitions

| Input State | Event Type | Thinking Tracker | Output |
|-------------|------------|------------------|--------|
| Empty | `content_block_start` (thinking) | Add index | Drop |
| Empty | `content_block_start` (text) | Unchanged | Forward |
| Has index | `content_block_delta` | Index in tracker | Drop |
| Has index | `content_block_delta` | Index not in tracker | Forward |
| Any | `message_start` | Unchanged | Forward (patch if needed) |
| Any | `[DONE]` | Unchanged | Forward |

## Notable Behaviors

1. **Index-based tracking**: Thinking blocks tracked by index, allowing correct filtering of all events (start, delta, stop) belonging to the same block

2. **Immediate flush**: Every forwarded event is flushed immediately to maintain real-time streaming behavior

3. **Malformed stream tolerance**: Trailing events without blank line terminator are still processed and forwarded

4. **Buffer auto-growth**: Scanner buffer starts at 4096 bytes but can grow up to 64 KiB for large events

5. **Patching independence**: Message start event patching operates independently from thinking block filtering; both can apply to same stream

6. **Minimal memory footprint**: Only current event accumulated in memory; previous events discarded after processing

7. **[DONE] token handling**: The `[DONE]` token is explicitly exempt from thinking block filtering. It is always forwarded regardless of whether a thinking block is active.

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Invalid JSON in event data | Pass through unchanged |
| Missing event type field | Forward unchanged |
| Missing index field | Not treated as thinking block |
| Scanner buffer overflow | Continue reading (buffer at max) |
| Read error during stream | Stop processing, close stream |

## Rationale

Local inference servers may not implement the Anthropic Messages API streaming format perfectly. Some servers:
- Omit required fields in `message_start` events
- Return thinking blocks without valid signatures
- Send malformed SSE streams without proper blank line terminators

This streaming handler compensates for these issues by:
- Filtering thinking blocks to prevent signature validation failures
- Patching missing fields in `message_start` events
- Tolerating malformed streams for robustness

The index-based tracking ensures all events belonging to thinking blocks are correctly identified and dropped, maintaining API compatibility with downstream consumers.
