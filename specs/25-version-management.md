# 25. Version Management and Reporting

## Topic Statement

Manage and report the detour proxy version through build-time configuration and runtime endpoints.

## Scope

**In-scope:**
- Version variable declaration and default value
- Version exposure via health endpoint
- Build-time version override mechanism

**Boundaries:**
- Input: Build configuration or runtime request
- Output: Version string in health response or compiled binary

## Data Contracts

### Version Variable

| Property | Value |
|----------|-------|
| Package | main (cmd/detour) and proxy (internal/proxy) |
| Default value | `"dev"` |
| Type | string |
| Exported | No (lowercase identifier) |

### Version Constants

| Location | Value | Usage |
|----------|-------|-------|
| `cmd/detour/main.go` | `var version = "dev"` | Not directly used, declared but health uses proxy's constant |
| `internal/proxy/handler.go` | `const version = "0.1.0"` | Used in health endpoint response |

## Behaviors

### Default Version Behavior

When binary is built without special flags:
- `internal/proxy/version` constant equals `"0.1.0"`
- Health endpoint returns `{"status":"ok","version":"0.1.0"}`

### Build-Time Version Override

Version can be overridden at build time using Go linker flags:

```bash
go build -ldflags "-X github.com/trydydd/detour/internal/proxy.version=1.2.3" -o detour ./cmd/detour
```

Or for main package version:

```bash
go build -ldflags "-X main.version=1.2.3" -o detour ./cmd/detour
```

**Note:** The `cmd/detour/main.go` declares `var version = "dev"` but does not use it directly. The health endpoint uses `internal/proxy.version` constant.

### Health Endpoint Version Reporting

When `/health` endpoint is called:
1. Handler constructs JSON object with `status: "ok"` and `version: "0.1.0"` (or overridden value)
2. Sets `Content-Type: application/json`
3. Encodes and returns JSON response

## State Transitions

| Build Configuration | Runtime Version | Health Response |
|---------------------|-----------------|-----------------|
| No ldflags | "0.1.0" | `{"status":"ok","version":"0.1.0"}` |
| `-X internal/proxy.version=2.0.0` | "2.0.0" | `{"status":"ok","version":"2.0.0"}` |

## Notable Behaviors

1. **Two version variables**: `cmd/detour/main.go` declares `var version = "dev"` but `internal/proxy/handler.go` uses `const version = "0.1.0"`. The main package version is not used anywhere in the codebase.

2. **Constant vs variable**: The proxy package uses a `const` which cannot be overridden at build time without recompilation with modified source. Only the main package `var` can be overridden via `-X` linker flag.

3. **Version immutability**: Once compiled, the version in the proxy package is fixed unless the binary is rebuilt with a different source or linker flag.

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Version variable empty | Returns empty string in health response |
| JSON encoding failure | Returns HTTP 500 with empty body (json.NewEncoder error not checked) |

## Rationale

Version reporting serves several purposes:

1. **Debugging**: Allows users and support to identify which version of detour is running
2. **Compatibility checking**: Clients can verify they are using compatible versions
3. **Release tracking**: Build-time version injection enables proper semantic versioning for releases

## Examples

### Default Build

```bash
go build -o detour ./cmd/detour
./detour --help
# Health endpoint returns: {"status":"ok","version":"0.1.0"}
```

### Versioned Build

```bash
go build -ldflags "-X github.com/trydydd/detour/internal/proxy.version=1.2.3" -o detour ./cmd/detour
# Health endpoint returns: {"status":"ok","version":"1.2.3"}
```

## Testing Scenarios

### Default Version
- Build without ldflags
- Call `/health` endpoint
- Expected: `version` field equals `"0.1.0"`

### Custom Version
- Build with `-X internal/proxy.version=custom-v1`
- Call `/health` endpoint
- Expected: `version` field equals `"custom-v1"`

### Version Format
- Version string can contain any characters valid in JSON string
- No validation of version format is performed
- Common formats: semantic versioning (1.2.3), pre-release (1.2.3-beta.1), git refs (v1.2.3-abc123)
