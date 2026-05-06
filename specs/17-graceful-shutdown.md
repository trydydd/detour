# 17. Graceful Shutdown and Signal Handling

## Topic Statement

Handle application shutdown gracefully when receiving interrupt signals, allowing active requests to complete before terminating.

## Scope

**In-scope:**
- Signal registration and handling
- Graceful shutdown sequence
- Connection timeout handling
- Subprocess lifecycle management

**Boundaries:**
- Input: Operating system signals (SIGINT)
- Output: Clean application termination
- Only handles graceful shutdown, not crash scenarios

## Data Contracts

### Signal Types

| Signal | Source | Effect |
|--------|--------|--------|
| SIGINT | Ctrl+C or kill -INT | Trigger graceful shutdown |

### Shutdown Timeout

- Maximum graceful shutdown duration: 3 seconds
- After timeout, force close remaining connections

## Behaviors

### Signal Registration

1. Create context with signal notification for SIGINT
2. Store signal handler function for later cancellation
3. Application continues normal operation while signal handler is active

### Shutdown Trigger

Graceful shutdown is triggered when:
1. User presses Ctrl+C (generates SIGINT)
2. External process sends SIGINT signal
3. Signal handler invokes the shutdown context cancellation

### Graceful Shutdown Sequence

1. Stop accepting new HTTP connections
2. Initiate server shutdown with 3-second timeout
3. Wait for active requests to complete naturally
4. If timeout expires, force close remaining connections
5. Application exits after shutdown completes

### Force Close Behavior

If graceful shutdown exceeds 3-second timeout:
1. Remaining active connections are forcibly closed
2. In-flight requests are terminated
3. Application proceeds to exit

### Subprocess Handling

During shutdown:
1. Claude subprocess may still be running
2. Parent process waits for subprocess to complete before exiting
3. If subprocess fails to launch, parent exits with error code 1
4. Exit code from subprocess is propagated to parent
5. Note: `srv.Shutdown()` is called but the returned context is not actively monitored for shutdown completion

## State Transitions

| State | Trigger | Next State |
|-------|---------|------------|
| Running | SIGINT received | Shutting down |
| Shutting down | All requests complete | Exited |
| Shutting down | 3-second timeout | Force close → Exited |
| Running | Subprocess launch fails | Exited (error) |

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Shutdown timeout | Force close connections, proceed to exit |
| Subprocess still running | Parent waits for subprocess completion |
| Subprocess launch error | Print error, exit with status 1 |

## Notable Behaviors

1. **Signal context is created but not explicitly used**: The signal context is created with `signal.NotifyContext` but the returned context is not actively monitored - shutdown relies on the `srv.Shutdown()` call

2. **Shutdown is fire-and-forget**: Once `srv.Shutdown()` is called, the application does not wait for its completion before checking subprocess status

3. **3-second timeout is hardcoded**: The shutdown timeout is fixed at 3 seconds and cannot be configured

4. **Subprocess blocks parent exit**: The parent process waits for the Claude subprocess to complete before exiting, even during shutdown

5. **No cleanup of partial state**: If shutdown is interrupted or force-closed, no explicit cleanup of partial state is performed

## Rationale

Graceful shutdown serves several purposes:

1. **Prevent request loss**: Allows in-flight requests to complete rather than being abruptly terminated
2. **Clean resource release**: Ensures HTTP server properly releases ports and file descriptors
3. **Subprocess coordination**: Provides opportunity for Claude subprocess to clean up before parent exits
4. **User experience**: Prevents abrupt disconnection when user presses Ctrl+C

The 3-second timeout balances between:
- Giving requests time to complete normally
- Preventing indefinite hangs during shutdown
- Allowing subprocess to complete its work

## Examples

### Normal Shutdown Sequence

```
User presses Ctrl+C
  ↓
Signal handler invoked
  ↓
srv.Shutdown() called with 3s timeout
  ↓
Server stops accepting new connections
  ↓
Active requests complete (or timeout)
  ↓
Subprocess exits
  ↓
Parent process exits
```

### Timeout Shutdown Sequence

```
User presses Ctrl+C
  ↓
Signal handler invoked
  ↓
srv.Shutdown() called with 3s timeout
  ↓
Server stops accepting new connections
  ↓
After 3 seconds, remaining connections forced closed
  ↓
Subprocess exits
  ↓
Parent process exits
```
