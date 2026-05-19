# 01. Configuration Management and Persistence

## Topic Statement

Load, save, validate, merge, and apply defaults to user configuration for proxy operation.

## Scope

**In-scope:**
- Configuration file storage and retrieval
- Command-line flag parsing and precedence
- Default value application
- Validation rules and error reporting
- Security permission checks on config files

**Boundaries:**
- Configuration is stored in user's home directory under a dedicated folder
- Configuration provides model name alias, API endpoint URL, and proxy port
- Output: validated configuration object ready for use

## Data Contracts

### Configuration Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `model_name` | string | Yes | — | Alias identifying the local model |
| `model_api` | string | Yes | — | Base URL of local inference server (http/https only) |
| `port` | integer | No | 8888 | Proxy listen port |

### Storage Format

JSON object persisted to `~/.detour/config.json` (or configurable directory):
```json
{
  "port": 8888,
  "model_name": "red",
  "model_api": "http://192.168.0.28:8000"
}
```

### File Permissions

- Saved files must have permissions `0600` (owner read/write only)
- Load operation rejects files with permissions more permissive than `0600`

## Behaviors

### Configuration Load Sequence

1. Attempt to read config file from storage directory
2. If file does not exist, return empty configuration (no error)
3. If file exists, verify permissions are not more permissive than `0600`
4. Parse JSON and decode into configuration structure
5. Return decoded configuration or error on parse failure

### Flag Merging Rules

Command-line flags merge with saved configuration:
- Non-zero flag values override saved values
- Zero-value flags (empty string, 0) leave saved values unchanged
- Merge priority: flags take precedence over saved configuration

### Default Application

Defaults applied after merging:
- Port: if value is 0, set to 8888

### Validation Rules

Configuration must satisfy all rules:
1. `model_name` must be non-empty
2. `model_api` must be non-empty
3. `model_api` must use `http` or `https` scheme only
4. `model_api` must not include path component (only scheme, host, and optional port)
5. `model_api` must not include query parameters
6. `model_api` must not include fragment
7. `model_api` must include a host

### Validation Error Messages

| Condition | Error Message |
|-----------|---------------|
| Missing model_name | "--model-name is required" |
| Missing model_api | "--model-api is required" |
| Invalid scheme | "model-api must use http or https scheme (got \"<scheme>\")" |
| Path present | "model-api must not include a path (got \"<path>\")" |
| Query present | "model-api must not include query parameters (got \"<query>\")" |
| Fragment present | "model-api must not include a fragment (got \"<fragment>\")" |
| No host | "model-api must include a host" |

### Config Save Sequence

1. Create storage directory if it does not exist (permissions `0755`)
2. Open config file with write/create/truncate mode
3. Set file permissions to `0600`
4. Encode configuration as JSON and write to file

### Cleartext Warning

When `model_api` uses `http://` scheme with a non-loopback host:
- Display warning about unencrypted transmission
- Suggest SSH tunnel or TLS termination as mitigation

Loopback addresses exempt from warning: `localhost`, `127.0.0.1`, `::1`

## State Transitions

| Operation | Input | Output | Side Effects |
|-----------|-------|--------|--------------|
| Load | Storage directory path | Configuration or error | None |
| Save | Configuration, directory path | Success or error | Creates directory, writes config file |
| Merge | Base config, flags config | Merged config | None |
| ApplyDefaults | Config | Config with defaults filled | None |
| Validate | Config | Error or nil | None |

## Notable Behaviors

1. Missing config file returns empty config, not error — allows first-run with only flags
2. File permission check on load prevents reading configs that could be modified by other users
3. Validation rejects all URL components except scheme, host, and port — API must be a base URL only
4. Cleartext warning only triggers for non-loopback HTTP endpoints
