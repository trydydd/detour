# 18. Configuration Directory Security

## Topic Statement

Enforce secure file permissions on configuration directory and files to prevent unauthorized access or modification.

## Scope

**In-scope:**
- Configuration directory creation with secure permissions
- Configuration file permission validation on load
- Configuration file permission setting on save
- Permission-based access control

**Boundaries:**
- Input: Configuration directory path and file operations
- Output: Secure configuration storage or permission errors
- Only applies to `~/.detour/` directory and its contents

## Data Contracts

### Configuration Directory

| Property | Value |
|----------|-------|
| Path | `~/.detour/` (user home directory) |
| Permissions | `0755` (owner read/write/execute, others read/execute) |

### Configuration File

| Property | Value |
|----------|-------|
| Path | `~/.detour/config.json` |
| Permissions | `0600` (owner read/write only) |
| Format | JSON |

## Behaviors

### Directory Creation

When creating the configuration directory:

1. Create directory at `~/.detour/` if it does not exist
2. Set directory permissions to `0755`
3. Allow owner full access (read, write, execute)
4. Allow others read and execute access (for directory traversal)

### Configuration File Save

When saving configuration:

1. Open file with flags: create if not exists, truncate if exists, write mode
2. Set file permissions to `0600` immediately upon file creation
3. Write JSON-encoded configuration to file
4. Close file handle
5. Permissions remain `0600` after write completes

### Configuration File Load

When loading configuration:

1. Check if configuration file exists
2. If file does not exist, return empty configuration (no error)
3. If file exists, stat the file to get permission bits
4. Extract permission bits (last 9 bits of mode)
5. Check if any permission bit beyond owner read/write is set:
   - Calculate: `perm & 0o077` (mask for group and other permissions)
   - If result is non-zero, file has insecure permissions
6. If permissions are more permissive than `0600`, return error
7. If permissions are `0600` or less permissive, proceed to read file
8. Parse JSON and return configuration

### Permission Check Logic

```
Insecure if: (file_permissions & 0o077) != 0

Where:
- file_permissions = os.ModePerm (lowest 9 bits)
- 0o077 = group read/write/execute + other read/write/execute
```

### Error Messages

| Condition | Error Message |
|-----------|---------------|
| Insecure permissions | "config file <path> has insecure permissions <perm> (want 0600)" |
| Cannot create directory | "mkdir <path>: <error>" |
| Cannot open file | OS-specific file error |
| Invalid JSON | "decode <path>: <error>" |

## State Transitions

| Operation | Input State | Output State | Permissions |
|-----------|-------------|--------------|-------------|
| Save | Directory exists | Config written | 0600 |
| Save | Directory missing | Directory + config created | Dir: 0755, File: 0600 |
| Load | File exists, perms 0600 | Config loaded | Unchanged |
| Load | File exists, perms >0600 | Error returned | Unchanged |
| Load | File does not exist | Empty config returned | N/A |

## Notable Behaviors

1. **Directory permissions are permissive (0755)**: The directory itself allows others to traverse it, but not modify its contents

2. **File permissions are restrictive (0600)**: Only the owner can read or write the configuration file

3. **Permission check uses bitwise AND**: The check `perm & 0o077 != 0` detects any group or other permissions

4. **Load rejects, does not fix**: If insecure permissions are detected, the load operation fails rather than attempting to fix permissions

5. **Save sets permissions explicitly**: File permissions are set to `0600` during the open operation, ensuring secure permissions on both new and existing files

## Security Rationale

Secure configuration file permissions serve several purposes:

1. **Protect API credentials**: The configuration may contain sensitive information such as API endpoints that should not be readable by other users

2. **Prevent tampering**: Restrictive write permissions prevent other users from modifying the configuration

3. **Defense in depth**: Even if system access controls are weakened, file permissions provide an additional layer of protection

4. **Compliance with security best practices**: Configuration files containing credentials should have restrictive permissions (typically 0600 or more restrictive)

## Examples

### Secure Permissions

```
File: ~/.detour/config.json
Permissions: -rw------- (0600)
Owner: user
Group: user

Result: Load succeeds, Save succeeds
```

### Insecure Permissions

```
File: ~/.detour/config.json
Permissions: -rw-r--r-- (0644)
Owner: user
Group: user

Result: Load fails with error "config file ~/.detour/config.json has insecure permissions 0644 (want 0600)"
```

### Group-Writable Permissions

```
File: ~/.detour/config.json
Permissions: -rw-rw---- (0660)
Owner: user
Group: users

Result: Load fails with error "config file ~/.detour/config.json has insecure permissions 0660 (want 0600)"
```

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Directory creation fails | Return error with mkdir details |
| File open fails | Return OS-specific error |
| Permission check fails | Return formatted error with path and permission value |
| JSON decode fails | Return error with decode details |
