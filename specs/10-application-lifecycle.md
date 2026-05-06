# 10. Application Lifecycle and Startup Sequence

## Topic Statement

Manage the complete lifecycle of the detour proxy application from startup through graceful shutdown.

## Scope

**In-scope:**
- Command-line argument parsing
- Configuration loading and merging
- Proxy server startup
- Subprocess launching
- Signal handling and graceful shutdown
- Error reporting

**Boundaries:**
- Input: Command-line arguments, environment
- Output: Proxy server operation, subprocess execution, exit status

## Data Contracts

### Command-Line Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--model-name` | string | Yes | — | Model alias sent to Claude Code |
| `--model-api` | string | Yes | — | Base URL of local inference server |
| `--port` | integer | No | 0 (dynamic) | Proxy listen port |
| Positional args | string array | No | — | Arguments passed to claude subprocess |

### Configuration Sources (in priority order)

1. Command-line flags (non-zero values)
2. Saved configuration file (`~/.detour/config.json`)
3. Built-in defaults

## Behaviors

### Application Startup Sequence

1. Parse command-line flags
2. Load saved configuration from `~/.detour/config.json` (if exists)
3. Merge flag values with saved configuration (flags take precedence)
4. Apply default values for zero-value fields
5. Validate merged configuration
6. Check for cleartext upstream warning and display if applicable
7. Save updated configuration to disk
8. Create HTTP proxy with registered handlers
9. Start proxy server in background goroutine
10. Wait for proxy to be ready (port listening)
11. Report proxy status to stderr
12. Launch claude subprocess with modified environment
13. Wait for subprocess completion
14. Gracefully shutdown proxy
15. Exit with subprocess exit status

### Configuration Merge Rules

- Non-zero flag values override saved configuration values
- Zero-value flags (empty string, 0) leave saved values unchanged
- Defaults applied after merging:
  - Port: 0 → 8888

### Validation Failures

If validation fails:
1. Print error message to stderr
2. Print usage hint to stderr
3. Exit with status code 1

### Proxy Startup Verification

1. Attempt to connect to proxy address using TCP dial with 100ms timeout
2. If connection succeeds, close the test connection and proceed
3. If connection fails, wait 50ms and retry
4. Continue retrying for up to 3 seconds total
5. On success: proceed to launch claude
6. On timeout: print error to stderr and exit with status 1

### Signal Handling

Registered signal handlers:
- `SIGINT` (Ctrl+C): Trigger graceful shutdown

Shutdown sequence:
1. Stop accepting new connections
2. Wait up to 3 seconds for active requests to complete
3. Force close remaining connections after timeout
4. Exit application

### Subprocess Launch

1. Create command with binary path (default: "claude")
2. Apply environment overrides:
   - `ANTHROPIC_BASE_URL`: Proxy URL
   - `ANTHROPIC_CUSTOM_MODEL_OPTION`: Model name alias
3. Attach stdin, stdout, stderr to parent process
4. Execute command
5. Block until subprocess exits
6. Return subprocess exit status

### Error Reporting

All errors printed to stderr with "detour:" prefix:
- Configuration errors: "detour: <error>"
- Proxy errors: "detour: proxy error: <error>"
- Launcher errors: "detour: <error>"
- Usage hint: "Usage: detour --model-name <alias> --model-api <url> [-- claude args]"

## State Transitions

| Phase | Input | Output | Side Effects |
|-------|-------|--------|--------------|
| Config Load | Directory path | Config or error | None |
| Config Merge | Base config, flags | Merged config | None |
| Validation | Config | Error or nil | None |
| Config Save | Config, directory | Success or error | Writes config file |
| Proxy Start | Config | HTTP server | Listens on port |
| Port Wait | Address, timeout | Ready or error | Polls connection |
| Subprocess Launch | Config, args | Exit status | Creates subprocess |
| Shutdown | Signal | Completion | Closes connections |

## Notable Behaviors

1. **Proxy starts before claude**: Proxy must be ready before launching claude to ensure no requests are lost
2. **Graceful shutdown on SIGINT**: Allows active requests to complete before exiting
3. **3-second shutdown timeout**: Forces termination if graceful shutdown takes too long
4. **Subprocess exit code propagated**: detour exits with same code as claude subprocess
5. **Warning for cleartext HTTP**: Alerts user when using http:// with non-loopback addresses
6. **Config auto-save**: Updated configuration saved after each run for convenience
7. **Positional args forwarded**: Any arguments after `--` passed directly to claude subprocess

## Version Information

- Current version: `0.1.0`
- Reported via `/health` endpoint
- Compiled from source (version variable can be set at build time)
