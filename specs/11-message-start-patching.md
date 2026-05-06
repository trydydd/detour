# 11. Message Start Event Patching

## Topic Statement

Inject missing `type` and `role` fields into `message_start` SSE events from local inference servers to maintain compatibility with downstream consumers.

## Scope

**In-scope:**
- Detection of `message_start` events in SSE streams
- Conditional injection of `type: "message"` and `role: "assistant"` into inner message object
- Preservation of existing fields during patching
- Pass-through of all other event types unchanged

**Boundaries:**
- Input: SSE stream from local inference server
- Output: SSE stream with patched `message_start` events
- Only applies to streaming responses from local backends (DoLocal path)
- Does not affect non-streaming responses

## Data Contracts

### Message Start Event Structure

Standard Anthropic `message_start` event:

```json
{
  "type": "message_start",
  "message": {
    "id": "string",
    "type": "message",
    "role": "assistant",
    "content": [],
    "model": "string",
    "stop_reason": null,
    "stop_sequence": null,
    "usage": {
      "input_tokens": integer,
      "output_tokens": integer
    }
  }
}
```

### vLLM-Shaped Event (Needs Patching)

Some local servers omit required fields:

```json
{
  "type": "message_start",
  "message": {
    "id": "chatcmpl-1",
    "content": [],
    "model": "red",
    "stop_reason": null,
    "usage": {
      "input_tokens": 1,
      "output_tokens": 0
    }
  }
}
```

Missing fields:
- `message.type` - should be `"message"`
- `message.role` - should be `"assistant"`

### Patched Event

```json
{
  "type": "message_start",
  "message": {
    "id": "chatcmpl-1",
    "type": "message",
    "role": "assistant",
    "content": [],
    "model": "red",
    "stop_reason": null,
    "usage": {
      "input_tokens": 1,
      "output_tokens": 0
    }
  }
}
```

## Behaviors

### Event Type Detection

For each SSE event line:

1. Parse the event data as JSON
2. Extract the `type` field from the outer event object
3. If `type` equals `"message_start"`, proceed to patching
4. If `type` is anything else, pass through unchanged

### Inner Message Extraction

For `message_start` events:

1. Extract the `message` object from the event
2. If `message` field is missing or not an object, skip patching
3. Parse the inner message object

### Field Injection Logic

For the inner message object:

1. Check if `type` field is present:
   - If missing, add `"type": "message"`
   - If present, leave unchanged
2. Check if `role` field is present:
   - If missing, add `"role": "assistant"`
   - If present, leave unchanged
3. Track whether any modifications were made

### Patch Decision

- If both `type` and `role` were already present: return original bytes unchanged, `changed = false`
- If at least one field was added: return modified JSON, `changed = true`

### JSON Re-serialization

When patching is required:

1. Marshal the modified message object back to JSON
2. Replace the original `message` field with the patched version
3. Marshal the entire event object back to JSON
4. Return the patched event data

### SSE Event Forwarding

After patching decision:

- If `changed = false`: forward original event lines byte-for-byte
- If `changed = true`: forward modified event with patched data
- For all non-`message_start` events: forward unchanged

## State Transitions

| Input Event | Missing Fields | Action | Output |
|-------------|----------------|--------|--------|
| `message_start` with complete message | None | No change | Original event |
| `message_start` missing `type` and `role` | Both | Add both | Patched event |
| `message_start` missing only `type` | `type` | Add `type` | Patched event |
| `message_start` missing only `role` | `role` | Add `role` | Patched event |
| `content_block_start` | N/A | Pass through | Unchanged |
| `content_block_delta` | N/A | Pass through | Unchanged |
| `content_block_stop` | N/A | Pass through | Unchanged |
| `message_delta` | N/A | Pass through | Unchanged |
| `message_stop` | N/A | Pass through | Unchanged |

## Notable Behaviors

1. **Minimal mutation**: Only modifies `message_start` events; all other SSE event types pass through byte-for-byte

2. **Idempotent**: If fields are already present, the event is returned unchanged — patching can be applied multiple times without side effects

3. **Preserves existing data**: All other fields in the message object (id, model, usage, etc.) are preserved during patching

4. **Required by downstream consumers**: Some systems (notably Claude Code's mobile transcript relay) require `type` and `role` fields and fail if they are missing

5. **Compensates for non-compliant servers**: Local inference servers like vLLM may omit these fields despite them being part of the Anthropic API specification

6. **Only applies to streaming**: Non-streaming responses from local servers do not undergo this patching — they are handled as complete JSON objects

7. **Patching happens after thinking block filtering**: The message_start patching operates independently of thinking block stripping; both transformations can apply to the same stream

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Invalid JSON in event data | Pass through unchanged |
| `message` field not an object | Skip patching, pass through |
| JSON marshaling failure | Return original bytes unchanged |

## Rationale

Some local inference servers (e.g., vLLM) implement the Messages API but omit required fields in `message_start` events. Downstream consumers that parse these streams expect `type: "message"` and `role: "assistant"` to be present in the inner message object. Without these fields, consumers may fail to process the stream correctly.

This patching behavior compensates for these non-compliant servers by injecting the missing fields, ensuring compatibility with downstream systems that expect fully compliant Anthropic API responses.
