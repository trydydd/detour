# 09. Mock LLM Test Server

## Topic Statement

Provide a mock HTTP server that emulates the Anthropic Messages API for testing purposes.

## Scope

**In-scope:**
- Mock `/v1/messages` endpoint with canned responses
- Mock `/v1/models` endpoint listing available models
- Streaming and non-streaming response modes
- Request validation and error handling

**Boundaries:**
- Input: HTTP requests conforming to Anthropic Messages API format
- Output: Mock responses in Anthropic API format
- No actual model inference or external API calls

## Data Contracts

### Request Format (Messages API)

```json
{
  "model": "string",
  "messages": [
    {"role": "user" | "assistant", "content": "string" | [{"type": "text", "text": "string"}]}
  ],
  "max_tokens": integer,
  "stream": boolean (optional)
}
```

### Response Format (Non-Streaming)

```json
{
  "id": "msg_mock_001",
  "type": "message",
  "role": "assistant",
  "content": [
    {"type": "text", "text": "THIS IS DETOUR TEST!"}
  ],
  "model": "<model_name>",
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 1,
    "output_tokens": 1
  }
}
```

### Response Format (Streaming - SSE)

Event sequence for streaming responses:

1. `message_start` - Initial message metadata
2. `content_block_start` - Start of content block (index 0)
3. `content_block_delta` - Text content delta
4. `content_block_stop` - End of content block
5. `message_delta` - Final message metadata with stop reason
6. `message_stop` - End of stream

## Behaviors

### Endpoint Registration

Mock server registers three handlers:

1. `/v1/messages` - POST only, handles message requests
2. `/v1/models` - GET only, returns list of mock models
3. `/` - Catch-all, returns 404 Not Found

### Messages Handler - Non-Streaming

For non-streaming requests:

1. Validate HTTP method is POST
2. Read request body
3. Parse JSON into request structure
4. Extract model name from request (use default if missing)
5. Construct mock response with:
   - Fixed ID: `msg_mock_001`
   - Type: `message`
   - Role: `assistant`
   - Content: Single text block with canned reply "THIS IS DETOUR TEST!"
   - Model: Request model or default
   - Stop reason: `end_turn`
   - Usage: 1 input token, 1 output token
6. Set `Content-Type: application/json`
7. Encode and send response

### Messages Handler - Streaming

For streaming requests (`stream: true`):

1. Validate HTTP method is POST
2. Read and parse request body
3. Extract model name from request (use default if missing)
4. Set headers:
   - `Content-Type: text/event-stream`
   - `Cache-Control: no-cache`
   - `Connection: keep-alive`
5. Emit SSE event sequence:
   - `message_start` with full message metadata including empty content array
   - `content_block_start` at index 0 with empty text block
   - `content_block_delta` at index 0 with full canned reply text
   - `content_block_stop` at index 0
   - `message_delta` with stop reason `end_turn` and output token count
   - `message_stop` indicating stream end
6. Flush after each event

### Message Start Event Structure

The `message_start` event includes complete message metadata:

```json
{
  "type": "message_start",
  "message": {
    "id": "msg_mock_001",
    "type": "message",
    "role": "assistant",
    "content": [],
    "model": "<model_name>",
    "stop_reason": null,
    "stop_sequence": null,
    "usage": {"input_tokens": 1, "output_tokens": 0}
  }
}
```

### Models Handler

For `/v1/models` requests:

1. Validate HTTP method is GET
2. Return list of three pre-defined models:
   - `claude-3-5-sonnet-20241022` (Claude 3.5 Sonnet)
   - `claude-3-opus-20250219` (Claude 3 Opus)
   - `claude-3-haiku-20240307` (Claude 3 Haiku)
3. Each model includes:
   - ID: Model identifier
   - Type: `model`
   - Created: Current Unix timestamp
   - Display Name: Human-readable name
   - Owned By: `anthropic`
4. Set `Content-Type: application/json`
5. Encode and send response

### Error Handling

| Condition | HTTP Status | Response Body |
|-----------|-------------|---------------|
| Wrong HTTP method on /v1/messages | 405 | `{"error": "method not allowed"}` |
| Wrong HTTP method on /v1/models | 405 | `{"error": "method not allowed"}` |
| Invalid JSON body | 400 | `{"error": "invalid JSON: <details>"}` |
| Unknown path | 404 | Plain text: "mockllm: not found" |

### Default Model Behavior

When request does not include `model` field:

- Response uses the configured default model name (default: `detour-mock`)
- Default configurable via `--model-name` flag at server startup

## Configuration

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 9999 | HTTP listen port |
| `--model-name` | "detour-mock" | Model tag in /v1/models response |
| `--host` | "127.0.0.1" | Bind address |

### Environment

No special environment variables required. The mock does not validate `ANTHROPIC_API_KEY`.

## State Transitions

| Input | Processing | Output |
|-------|------------|--------|
| POST /v1/messages (non-stream) | Parse, construct response | JSON message response |
| POST /v1/messages (stream) | Parse, emit SSE sequence | SSE event stream |
| GET /v1/models | Construct model list | JSON models list |
| Any other path | None | 404 Not Found |

## Notable Behaviors

1. **Canned reply**: All responses contain identical text "THIS IS DETOUR TEST!" regardless of input
2. **Token counts**: Always reports 1 input token and 1 output token
3. **Complete message_start**: Unlike some real inference servers, includes `type` and `role` fields in message_start event
4. **No authentication**: Does not validate or require API keys
5. **Stateless**: Each request handled independently with no session state
6. **Stop reason fixed**: Always returns `end_turn` as stop reason
