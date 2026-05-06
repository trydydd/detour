# 05. Thinking Block Stripping for Local Inference

## Topic Statement

Remove thinking content blocks from requests and responses when routing to local inference servers to maintain compatibility with Anthropic API signature requirements.

## Scope

**In-scope:**
- Removal of `thinking` field from request bodies
- Removal of thinking content blocks from response bodies (JSON and streaming)
- Filtering of thinking-related beta tokens from headers
- Streaming event filtering for thinking blocks

**Boundaries:**
- Input: JSON request/response bodies or SSE stream
- Output: Modified JSON or SSE stream without thinking content
- Only applies to local routing, never to passthrough

## Data Contracts

### Thinking Field in Request

```json
{
  "thinking": {
    "type": "enabled",
    "budget_tokens": 5000
  }
}
```

### Thinking Block in Response Content

```json
{
  "type": "thinking",
  "thinking": "reasoning text",
  "signature": "signature string"
}
```

### Thinking Beta Token

```
interleaved-thinking-2025-05-14
```

## Behaviors

### Request Body Transformation

Strip thinking from JSON request body:

1. Parse request body as JSON object
2. If `thinking` field present, delete it
3. Re-serialize to JSON
4. Return modified body

### Non-Streaming Response Transformation

Remove thinking blocks from JSON response:

1. Parse response as JSON object
2. Extract `content` array from response
3. Iterate through content blocks
4. For each block, check if `type` equals "thinking"
5. Exclude thinking blocks from output array
6. Re-serialize content array to JSON
7. Replace original content with filtered array
8. Re-serialize entire response
9. Return modified body

### Streaming Response Transformation

Filter thinking events from SSE stream:

1. Initialize empty thinking block index tracker
2. Parse stream line by line, grouping into events (blank line separates events)
3. For each complete event:
   - If event type is `content_block_start` with `content_block.type == "thinking"`:
     - Record the block index as thinking
     - Skip this event (do not forward)
   - If event type is `content_block_delta` or `content_block_stop`:
     - Check if index is in thinking tracker
     - If yes, skip this event
     - If no, forward event
   - For all other event types: forward unchanged
4. Flush after each forwarded event

### Beta Header Filtering

Remove thinking tokens from `Anthropic-Beta` header:

1. Split header value by comma
2. Trim whitespace from each token
3. Exclude tokens containing "thinking" substring
4. Join remaining tokens with comma
5. Return filtered header value

### Message Start Event Patching

For streaming responses from local servers, inject missing fields:

1. Parse each SSE event data as JSON
2. If event type is `message_start`:
   - Extract inner `message` object
   - If `message.type` missing, add `"type": "message"`
   - If `message.role` missing, add `"role": "assistant"`
   - Re-serialize and replace original event data
3. All other events pass through unchanged

## State Transitions

| Input | Transformation | Output |
|-------|----------------|--------|
| Request with thinking field | Delete field | Request without thinking |
| Response with thinking blocks | Filter array | Response without thinking blocks |
| SSE stream with thinking events | Drop events | Stream without thinking events |
| Beta header with thinking token | Remove token | Filtered header |
| message_start missing type/role | Inject fields | Patched event |

## Notable Behaviors

1. Thinking blocks stripped because local inference servers cannot produce valid Anthropic signatures; blocks with invalid signatures cause subsequent passthrough requests to fail with 400 invalid-signature error
2. Message start patching compensates for some local servers (e.g., vLLM) that omit `type` and `role` fields required by downstream consumers
3. Streaming filter tracks thinking block indices to correctly drop all events belonging to thinking blocks (start, delta, stop)
4. Patching only modifies message_start events; all other event types pass through byte-for-byte
