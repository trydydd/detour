# 06. Environment Injection and Subprocess Launching

## Topic Statement

Launch Claude Code as a subprocess with modified environment variables that redirect API calls through the detour proxy.

## Scope

**In-scope:**
- Environment variable construction from base environment and overrides
- Subprocess creation with custom environment
- Standard I/O passthrough
- Exit code propagation

**Boundaries:**
- Input: configuration object, command-line arguments
- Output: subprocess execution result
- Does not include proxy startup (handled separately)

## Data Contracts

### Environment Overrides

| Variable | Value | Purpose |
|----------|-------|---------|
| `ANTHROPIC_BASE_URL` | `http://127.0.0.1:<port>` | Redirect API calls to detour proxy |
| `ANTHROPIC_CUSTOM_MODEL_OPTION` | `<model-name>` | Set custom model alias for Claude Code |

### Proxy URL Construction

Format: `http://127.0.0.1:<port>`

Where `<port>` is the configured proxy listen port.

## Behaviors

### Environment Construction Sequence

1. Start with current process environment
2. For each base environment variable:
   - If variable key appears in overrides, replace with override value
   - If override value is empty string, exclude variable from output
   - Otherwise, keep original value
3. For each override key not in base environment:
   - Add override key-value pair to output
4. Return complete environment array

### Subprocess Launch Sequence

1. Create command with specified binary and arguments
2. Apply constructed environment to command
3. Attach standard input from parent process
4. Attach standard output to parent process
5. Attach standard error to parent process
6. Execute command and wait for completion
7. Return subprocess exit status or error

### Default Binary

If no binary path specified, use `"claude"` as default.

### Exit Code Handling

- Subprocess exit code 0: return nil error
- Subprocess non-zero exit: return error wrapping subprocess failure

## State Transitions

| Input | Process | Output |
|-------|---------|--------|
| Base env + overrides | Merge with priority to overrides | Complete environment |
| Config + args + env | Launch subprocess | Exit status |

## Notable Behaviors

1. Existing `ANTHROPIC_API_KEY` preserved in environment — detour does not modify authentication credentials
2. Empty override values cause variable exclusion — allows unsetting environment variables
3. Standard I/O passthrough enables interactive terminal usage
4. Exit code propagation ensures parent process can detect subprocess failure
