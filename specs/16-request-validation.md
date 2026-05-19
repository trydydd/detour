# 16. Request Validation and Model Field Extraction

## Topic Statement

Validate incoming HTTP requests to the Messages API endpoint and extract the model field for routing decisions.

## Scope

**In-scope:**
- Request body reading with size limits
- JSON parsing and error handling
- Model field extraction from request body
- Validation error responses

**Boundaries:**
- Input: HTTP POST requests to `/v1/messages`
- Output: Validated request data or error response
- Size limiting is enforced before validation (see spec 13)

## Data Contracts

### Request Body Shape

```json
{
  "model": "string",
  "messages": "array",
  "max_tokens": "integer",
  "thinking": "object (optional)",
  "stream": "boolean (optional)"
}
```

### Model Field Requirements

- Must be present in the JSON object
- Must be a non-empty string
- Used to determine routing destination (local vs passthrough)

## Behaviors

### Request Body Reading

1. Read request body using `io.LimitReader` with limit of `maxRequestBytes + 1` (10,485,761 bytes)
2. Store read bytes in memory for later processing
3. If read operation returns error, immediately return 400 error

### Size Validation

1. After reading, check if actual body length exceeds `maxRequestBytes` (10 MiB)
2. If exceeded, return 413 error before any JSON parsing
3. If within limit, proceed to JSON parsing

### JSON Parsing

1. Attempt to unmarshal the read bytes into a generic JSON object structure
2. If parsing fails, return 400 error with parse error details
3. On success, extract the `model` field value

### Model Field Validation

1. Check if `model` field exists in parsed JSON
2. Check if `model` field value is a non-empty string
3. If missing or empty, return 400 error with message "missing required field: model"

### Error Response Format

All validation errors use the standard error response format:

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request",
    "message": "<specific error message>"
  }
}
```

## Error Conditions

| Condition | HTTP Status | Error Message |
|-----------|-------------|---------------|
| Body read failure | 400 | "could not read request body" |
| Body exceeds 10 MiB | 413 | "request body too large" |
| Invalid JSON syntax | 400 | JSON parse error details |
| Missing model field | 400 | "missing required field: model" |
| Empty model field | 400 | "missing required field: model" |

## State Transitions

| Input State | Action | Output |
|-------------|--------|--------|
| Body read error | Return immediately | 400 error |
| Body > 10 MiB | Return immediately | 413 error |
| Invalid JSON | Return immediately | 400 parse error |
| Missing model | Return immediately | 400 missing field error |
| Valid request | Continue processing | Model value extracted |

## Notable Behaviors

1. **Two-pass read**: Body is read once into memory, then parsed twice - once to extract model, then again for full processing

2. **Early rejection**: Validation errors are returned immediately without attempting any forwarding or transformation

3. **Model extraction uses generic parsing**: The model field is extracted using a minimal struct that only contains the model field, avoiding full request deserialization at this stage

4. **Error messages preserve parser details**: JSON parse errors include the underlying parser error message for debugging

5. **Validation precedes routing**: Model field must be successfully extracted before routing decision can be made

## Rationale

Request validation serves several critical purposes:

1. **Prevent invalid requests**: Ensures only properly formatted requests proceed to routing and forwarding
2. **Clear error feedback**: Provides specific error messages to help clients understand what went wrong
3. **Security**: Validates input before any processing to prevent injection or malformed data attacks
4. **Routing prerequisite**: The model field is required to make routing decisions, so it must be validated first

The validation sequence (read → size check → JSON parse → model extract) ensures errors are caught at the earliest possible stage, minimizing resource usage and providing clear error boundaries.
