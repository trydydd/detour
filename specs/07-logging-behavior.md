# 07. Logging Behavior

## Topic Statement

Conditionally log HTTP requests to stderr when logging is enabled via environment variable.

## Scope

**In-scope:**
- Log enablement detection from environment
- Request logging format and output
- Log line structure

**Boundaries:**
- Input: `DETOUR_LOG` environment variable, HTTP requests
- Output: stderr log lines
- No log persistence or file output

## Data Contracts

### Environment Variable

| Variable | Truthy Values | Effect |
|----------|---------------|--------|
| `DETOUR_LOG` | `1`, `true`, `yes`, `on` (case-insensitive) | Enable logging |
| `DETOUR_LOG` | Empty or any other value | Disable logging |

### Log Line Format

```
detour: <METHOD> <PATH> <STATUS> <DURATION>
```

Where:
- `METHOD`: HTTP method (GET, POST, etc.)
- `PATH`: Request URI path
- `STATUS`: HTTP status code (integer)
- `DURATION`: Request duration in milliseconds (rounded)

### Example Log Lines

```
detour: POST /v1/messages 200 150ms
detour: GET /health 200 1ms
detour: POST /v1/messages 400 5ms
```

## Behaviors

### Log Enablement Detection

Parse `DETOUR_LOG` environment variable:

1. Read environment variable value
2. Convert to lowercase
3. Check if value equals `1`, `true`, `yes`, or `on`
4. Return true if match, false otherwise

### Request Logging Sequence

When logging enabled:

1. Record start timestamp
2. Execute wrapped handler
3. Capture HTTP status code from response
4. Calculate duration from start timestamp
5. Round duration to nearest millisecond
6. Write log line to stderr

### Status Code Capture

Track status code through response writer:

- Default status: 200
- Updated when `WriteHeader` called
- Only first call to `WriteHeader` recorded

## State Transitions

| DETOUR_LOG Value | Logging Enabled |
|------------------|-----------------|
| (empty) | No |
| `0` | No |
| `false` | No |
| `no` | No |
| `off` | No |
| `random` | No |
| `1` | Yes |
| `true` | Yes |
| `True` | Yes |
| `YES` | Yes |
| `on` | Yes |

## Notable Behaviors

1. Logging disabled by default — no performance impact when not explicitly enabled
2. One line per request — minimal verbosity for debugging
3. Duration rounded to milliseconds — human-readable output
4. Case-insensitive truthy value matching — flexible configuration
