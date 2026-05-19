# 24. Subprocess Lifecycle and Exit Code Handling

## Topic Statement

Launch and manage Claude Code as a subprocess with environment injection and exit status handling.

## Scope

**In-scope:**
- Subprocess creation with custom environment
- Standard I/O passthrough to parent process
- Exit status handling and error reporting

**Boundaries:**
- Input: Configuration object, command-line arguments, optional binary override
- Output: Subprocess execution result (error or nil)
- Does not include proxy operation (separate spec)

## Data Contracts

### Launch Function Signature

```
Launch(cfg *Config, claudeArgs []string, claudeBin string) error
```

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| cfg | *config.Config | Yes | — | Proxy configuration containing port and model name |
| claudeArgs | []string | No | nil | Arguments to pass to claude subprocess |
| claudeBin | string | No | "claude" | Path to claude binary (empty uses default) |

### Environment Overrides

| Variable | Value Format | Purpose |
|----------|--------------|---------|
| `ANTHROPIC_BASE_URL` | `http://127.0.0.1:<port>` | Redirect API calls to detour proxy |
| `ANTHROPIC_CUSTOM_MODEL_OPTION` | `<model-name>` | Set custom model alias for Claude Code |

## Behaviors

### Subprocess Launch Sequence

1. If `claudeBin` is empty string, use default value `"claude"`
2. Build environment by:
   - Starting with current process environment (`os.Environ()`)
   - For each base environment variable:
     - If variable key appears in overrides, replace with override value
     - If override value is empty string, exclude variable from output
     - Otherwise, keep original value
   - For each override key not in base environment:
     - Add override key-value pair to output
3. Create command with binary path and arguments
4. Apply constructed environment to command
5. Attach standard input from parent process
6. Attach standard output to parent process
7. Attach standard error to parent process
8. Execute command and wait for completion
9. Return subprocess exit status or error

### Exit Code Handling

- Subprocess exit code 0: return nil error
- Subprocess non-zero exit: return error wrapping subprocess failure
- Note: The error returned wraps the exit status but does not directly expose the numeric exit code to caller

### Error Reporting

When launch fails:
1. Error is printed to stderr by caller (main function)
2. Caller exits with status code 1 regardless of subprocess exit code
3. Original subprocess exit code is not propagated to parent process exit status

## State Transitions

| Input | Process | Output |
|-------|---------|--------|
| Config + args + env | Launch subprocess | Exit status (wrapped in error if non-zero) |
| Binary not found | Exec failure | Error with "executable file not found" |
| Permission denied | Exec failure | Error with permission denied details |

## Notable Behaviors

1. **Standard I/O passthrough**: Enables interactive terminal usage by directly connecting subprocess I/O to parent process I/O

2. **Blocking execution**: `cmd.Run()` blocks until subprocess completes; parent does not proceed until subprocess exits

3. **Exit code loss**: While the error from `cmd.Run()` contains exit status information, the caller (main) only checks for error presence and exits with code 1, losing the original subprocess exit code

4. **Environment merging**: Overrides take precedence over base environment; empty override values cause variable exclusion

5. **Default binary**: If no binary path specified, uses `"claude"` as default - assumes claude is in PATH

## Rationale

The subprocess launch behavior serves several purposes:

1. **Seamless integration**: By inheriting stdin/stdout/stderr, the subprocess operates as if launched directly from terminal

2. **Environment injection**: Modifies only the necessary environment variables to redirect API calls through detour

3. **Error propagation**: Allows parent to detect subprocess failure and exit appropriately

4. **Flexibility**: Optional binary override enables testing with mock binaries or custom paths

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Binary not found | Return error with "executable file not found" details |
| Permission denied | Return error with permission denied details |
| Subprocess exits non-zero | Return error wrapping exit status |
| Signal interruption | Depends on OS signal handling (typically terminated) |

## Testing Scenarios

### Normal Launch
- Input: Valid config, no args, default binary
- Expected: Subprocess launches, inherits environment, exits normally

### Environment Injection
- Input: Config with port 8888, model name "red"
- Expected: `ANTHROPIC_BASE_URL=http://127.0.0.1:8888` in subprocess environment
- Expected: `ANTHROPIC_CUSTOM_MODEL_OPTION=red` in subprocess environment

### Argument Forwarding
- Input: Args `["--foo", "bar"]`
- Expected: Subprocess receives arguments `--foo bar`

### Exit Code Handling
- Input: Binary that exits with code 2
- Expected: `Launch()` returns non-nil error
- Note: Caller exits with status 1, not 2 (exit code not propagated)

## Implementation Notes

The `launchCapture()` function exists as a test helper that captures subprocess stdout/stderr to a buffer instead of passthrough. This function is not part of the public API and is only used in tests.
