# System Prompt Injection Implementation Plan

## Purpose

Add runtime-controlled system prompt injection to Detour by introducing an optional `--system-prompt-file` parameter. When provided, Detour should read a local text file at startup and inject its contents into Anthropic Messages API requests routed through the proxy. When omitted, Detour behavior must remain unchanged.

This plan is intentionally detailed so a coding agent with a roughly 50k token context window can implement each task independently. Each task includes an objective, scoped files, implementation notes, measurable success indicators, and suggested tests.

## Current Architecture Summary

Detour is a Go CLI that:

1. Parses CLI flags in `cmd/detour/main.go`.
2. Loads and saves persistent configuration through `internal/config`.
3. Starts a local HTTP proxy through `internal/proxy`.
4. Routes `/v1/messages` requests in `internal/proxy/handler.go`.
5. Sends requests for the configured model alias to a local upstream and sends other traffic to Anthropic.
6. Launches Claude Code with environment variables pointing it at the Detour proxy.

Relevant current request flow:

```text
Claude Code
  -> Detour proxy /v1/messages
    -> read request body
    -> peek model
    -> route based on configured alias
    -> for local route: strip thinking request field and filter thinking beta header
    -> forward to local or Anthropic upstream
```

System prompt injection should fit into the `/v1/messages` body transformation step before the request is forwarded.

## Target User Experience

### CLI examples

```bash
# Existing behavior: no injection
detour --model-name qwen --model-api http://127.0.0.1:8000 -- claude

# New behavior: inject prompt from file
detour --model-name qwen --model-api http://127.0.0.1:8000 --system-prompt-file ./prompts/detour-system.md -- claude
```

### Expected runtime behavior

- `--system-prompt-file` is optional.
- If omitted or empty, requests are forwarded exactly as they are today, aside from existing transformations.
- If provided, Detour reads the file once during startup.
- If the file cannot be read, is a directory, is invalid, or exceeds the configured size limit, Detour exits before starting the proxy.
- The file contents are injected into the Messages API `system` field for `/v1/messages` requests.
- Injection applies to both local and passthrough `/v1/messages` requests unless a task explicitly changes this behavior after product review.
- Injection must not affect `/v1/models`, `/health`, or passthrough non-message routes.

## Product Semantics

### Anthropic Messages `system` shape to support

Messages API requests can encode the system prompt in at least these forms:

1. No `system` field.
2. `system` as a JSON string.
3. `system` as an array of content blocks.

The implementation should preserve valid existing system content and add the injected content in a deterministic place.

### Injection merge policy

Use this default merge policy unless product requirements change:

| Existing request body | Result after injection |
| --- | --- |
| No `system` field | Add `system` as an array containing one text block with injected prompt. |
| `system` is a string | Convert to an array: first a text block for injected prompt, then a text block for the original string. |
| `system` is an array | Prepend one text block for injected prompt to the existing array. |
| `system` exists but is not string or array | Return HTTP 400 `invalid_request` with clear message; do not forward. |

Injected block shape:

```json
{
  "type": "text",
  "text": "<file contents>"
}
```

Rationale for prepending: a Detour-level system prompt is an operator-controlled instruction layer and should appear before application-provided system content. The implementation must document this choice in tests and code comments.

### Whitespace policy

- Preserve file contents exactly except normalize a UTF-8 byte order mark if present by removing it from the beginning of the file.
- Do not trim trailing newlines; prompts often intentionally include final newlines.
- Reject empty files after BOM removal. An all-whitespace file is allowed only if it has at least one byte after BOM removal; however, tests should include an empty-file rejection.

### File size and encoding policy

- Maximum system prompt file size: 256 KiB.
- Files larger than the limit must cause startup validation to fail before the proxy starts.
- Treat file contents as UTF-8 text. Reject invalid UTF-8 with a clear error.
- Read regular files only. Reject directories and non-regular files.

## Security and Privacy Considerations

- The system prompt file may contain sensitive operational instructions. Do not log the prompt contents.
- Error messages may include the file path but must not include file contents.
- Persisting the file path is acceptable if it follows the existing config pattern, but persisting the prompt contents is not acceptable.
- Avoid reading the file per request to prevent request latency spikes, time-of-check/time-of-use surprises, and accidental prompt changes during a running session.
- Do not accept remote URLs for `--system-prompt-file`; it must be a filesystem path.
- The proxy currently limits request bodies to 10 MiB. The injected prompt can increase outbound body size, but the inbound limit should remain applied before transformation.

## Recommended Implementation Strategy

Implement this as a small, testable request-body transformation in `internal/proxy`, with config and CLI plumbing separated from JSON mutation logic.

High-level steps:

1. Extend `config.Config` with `SystemPromptFile string` and `SystemPrompt string` or a similarly named in-memory field.
2. Add `--system-prompt-file` CLI flag in `cmd/detour/main.go`.
3. Load and validate the prompt file after merging config and before starting the proxy.
4. Pass the loaded prompt text into `proxy.Config`.
5. Add a focused body transformation helper in `internal/proxy` that injects the prompt into Messages API request JSON.
6. Call the helper in `makeMessagesHandler` after `peekModel` succeeds and before local thinking stripping / forwarding.
7. Add unit and integration tests for config, CLI behavior, and proxy body transformation.
8. Update user-facing README documentation.

## Detailed Tasks

### Task 1: Define configuration model and persistence behavior

**Objective:** Add a first-class configuration field for the optional system prompt file path without storing prompt contents in the persisted config.

**Primary files:**

- `internal/config/config.go`
- `internal/config/config_test.go`

**Implementation notes:**

- Add `SystemPromptFile string` with JSON tag `system_prompt_file,omitempty` to `Config`.
- Add an in-memory-only field for loaded prompt contents, for example `SystemPrompt string `json:"-"``. If an in-memory field is not placed on `Config`, define where else it lives and keep the no-persistence rule explicit.
- Update `merge` logic so a non-empty CLI flag overrides saved config.
- Decide whether an empty CLI value can clear a saved value. Current flag-merging style cannot distinguish omitted from explicitly empty strings. Do not solve this unless necessary; document that clearing can be done by editing `~/.detour/config.json` if current config behavior persists optional values.
- Keep `Validate()` focused on structural config validation. Do not read files in `Validate()` unless all existing call sites are updated and tests make startup behavior clear.

**Measurable indicators of success:**

- `Config` can marshal/unmarshal `system_prompt_file`.
- Loaded prompt contents never appear in `config.json` serialization.
- Existing config tests continue to pass.
- New tests prove `MergeFlags` preserves saved prompt file path and lets a non-empty flag override it.

**Suggested tests:**

- `TestMergeFlagsSystemPromptFileOverride`
- `TestConfigSaveDoesNotPersistSystemPromptContents`
- `TestLoadConfigWithSystemPromptFile`

**Context budget guidance:** This task should fit in one 50k token context by loading only `internal/config/config.go`, `internal/config/config_test.go`, and any compile errors from `go test ./internal/config`.

---

### Task 2: Implement prompt file loading and validation

**Objective:** Create a reusable prompt-file loader that reads the file once, validates size/type/encoding, removes an optional UTF-8 BOM, and returns prompt text.

**Primary files:**

- Recommended new file: `internal/config/system_prompt.go`
- Recommended new test file: `internal/config/system_prompt_test.go`

**Implementation notes:**

- Define a constant such as `const MaxSystemPromptBytes = 256 << 10`.
- Use `os.Stat` or `os.Lstat` plus mode checks to reject directories and non-regular files.
- Read at most `MaxSystemPromptBytes + 1` bytes, preferably with `io.LimitReader`.
- Return an error if more than `MaxSystemPromptBytes` bytes are present.
- Remove UTF-8 BOM prefix `[]byte{0xEF, 0xBB, 0xBF}` if present.
- Reject an empty prompt after BOM removal.
- Reject invalid UTF-8 using `utf8.Valid`.
- Return string contents without trimming whitespace.
- Keep errors actionable and free of prompt contents.

**Measurable indicators of success:**

- Loader accepts a normal UTF-8 file and returns exact content including trailing newline.
- Loader rejects missing files, directories, non-regular files if practical to test, empty files, oversized files, and invalid UTF-8.
- Loader removes BOM only at the start of the file.
- No tests print or compare errors containing prompt contents.

**Suggested tests:**

- `TestLoadSystemPromptFilePreservesContent`
- `TestLoadSystemPromptFileRejectsMissing`
- `TestLoadSystemPromptFileRejectsDirectory`
- `TestLoadSystemPromptFileRejectsEmpty`
- `TestLoadSystemPromptFileRejectsOversized`
- `TestLoadSystemPromptFileRejectsInvalidUTF8`
- `TestLoadSystemPromptFileRemovesBOM`

**Context budget guidance:** This task should fit in one 50k token context by loading only the config package files and running `go test ./internal/config`.

---

### Task 3: Add CLI flag plumbing and startup failure behavior

**Objective:** Add `--system-prompt-file` to the Detour CLI and ensure prompt-file problems fail fast before the proxy starts or Claude Code launches.

**Primary files:**

- `cmd/detour/main.go`
- `cmd/detour/main_test.go`
- `internal/config/config.go`

**Implementation notes:**

- Register the flag with a clear help string, for example: `path to UTF-8 system prompt file to inject into /v1/messages requests (optional)`.
- After `config.MergeFlags`, `ApplyDefaults`, and `Validate`, load the prompt file only if `cfg.SystemPromptFile != ""`.
- Store loaded contents in an in-memory field or local variable and pass it to `proxy.Config`.
- Error format should be user-actionable, for example: `detour: load system prompt file: <reason>`.
- Startup must not create the HTTP server if prompt loading fails.
- Existing launch behavior should remain unchanged when the flag is absent.

**Measurable indicators of success:**

- `detour --help` shows `--system-prompt-file`.
- Missing or invalid prompt file causes non-zero exit before proxy startup.
- Valid prompt file allows startup to continue.
- The prompt text is passed into `proxy.Config` exactly once during startup.
- Existing CLI tests continue to pass.

**Suggested tests:**

- If existing tests invoke `main` behavior, add coverage there.
- If direct `main` testing is awkward, create smaller helper functions for flag parsing or startup config preparation and test those helpers.
- Assert that `proxy.Config.SystemPrompt` is set from a valid prompt file in a helper-level test.

**Context budget guidance:** This task should fit in one 50k token context by loading `cmd/detour/main.go`, `cmd/detour/main_test.go`, `internal/config` files, and compile errors from `go test ./cmd/detour ./internal/config`.

---

### Task 4: Add proxy configuration field and JSON injection helper

**Objective:** Implement deterministic Messages API system prompt injection as a pure body transformation with comprehensive unit tests.

**Primary files:**

- `internal/proxy/handler.go`
- Recommended new file: `internal/proxy/system_prompt.go`
- Recommended new test file: `internal/proxy/system_prompt_test.go`

**Implementation notes:**

- Add `SystemPrompt string` or equivalent to `proxy.Config`.
- Implement a helper similar to:

```go
func injectSystemPrompt(body []byte, prompt string) ([]byte, error)
```

- If `prompt == ""`, return the original body unchanged and no error.
- Parse body into `map[string]json.RawMessage` to preserve unrelated fields.
- If the JSON body is invalid, let the existing `peekModel` behavior handle it. Since the handler already parses via `peekModel`, call injection after `peekModel` to avoid duplicate error messages where possible.
- Support missing `system`, string `system`, and array `system` as defined in the merge policy.
- For invalid `system` types, return an error that the handler maps to HTTP 400 `invalid_request`.
- Marshal the final request body using Go JSON encoding. Byte-for-byte preservation is not required when injection is active.
- Avoid logging prompt contents.

**Measurable indicators of success:**

- With empty prompt, helper returns the exact original byte slice or byte-equivalent content without mutation.
- With missing `system`, output has `system` as a one-element text block array.
- With string `system`, output has injected prompt block before original string converted to a text block.
- With array `system`, output prepends injected prompt and preserves existing array elements.
- With object/number/bool/null `system`, helper returns an error and handler returns HTTP 400.
- Unrelated request fields such as `model`, `messages`, `max_tokens`, `stream`, and `thinking` remain present.

**Suggested tests:**

- `TestInjectSystemPromptNoPromptNoop`
- `TestInjectSystemPromptAddsMissingSystem`
- `TestInjectSystemPromptPrependsStringSystem`
- `TestInjectSystemPromptPrependsArraySystem`
- `TestInjectSystemPromptRejectsInvalidSystemTypes`
- `TestInjectSystemPromptPreservesUnrelatedFields`

**Context budget guidance:** This task should fit in one 50k token context by loading only `internal/proxy/handler.go`, relevant proxy tests, and new helper files.

---

### Task 5: Integrate injection into `/v1/messages` request flow

**Objective:** Apply loaded system prompt contents to every `/v1/messages` request before forwarding, without changing routes that should not be affected.

**Primary files:**

- `internal/proxy/handler.go`
- `internal/proxy/handler_test.go`

**Implementation notes:**

- Call `injectSystemPrompt` after `peekModel(body)` succeeds and before route-specific body modifications.
- Recommended order:
  1. Read and size-limit inbound body.
  2. Parse/peek model and validate model presence.
  3. Inject system prompt if configured.
  4. Determine route or keep current route call after model peek.
  5. For local route only, strip thinking and filter thinking beta header.
  6. Forward.
- If injection returns an error, call `writeError(w, http.StatusBadRequest, "invalid_request", err.Error())` and return.
- Injection should run for both passthrough and local messages requests so behavior is model-route independent.
- Do not inject into `/v1/models` or generic passthrough routes.

**Measurable indicators of success:**

- Local `/v1/messages` upstream receives injected system prompt.
- Passthrough `/v1/messages` upstream receives injected system prompt.
- `/v1/models` upstream does not receive a request body modification.
- Invalid `system` type produces HTTP 400 and no upstream request.
- Existing thinking-stripping behavior still occurs for local route after injection.
- Existing non-injection tests pass unchanged or with intentional fixture updates.

**Suggested tests:**

- `TestMessagesInjectsSystemPromptForLocalRoute`
- `TestMessagesInjectsSystemPromptForPassthroughRoute`
- `TestMessagesDoesNotInjectWhenPromptEmpty`
- `TestMessagesRejectsInvalidSystemPromptShape`
- `TestMessagesInjectionDoesNotBreakThinkingStrip`

**Context budget guidance:** This task should fit in one 50k token context by loading only proxy package files and running `go test ./internal/proxy`.

---

### Task 6: End-to-end and launcher-adjacent validation

**Objective:** Verify the feature works across CLI config preparation, proxy startup, and request forwarding without changing Claude launcher behavior unexpectedly.

**Primary files:**

- `cmd/detour/main_test.go`
- `internal/launcher/launcher_test.go`
- Existing tests under `internal/proxy` and `internal/config`

**Implementation notes:**

- Launcher environment variables should not need changes unless a design decision requires passing the prompt file path to Claude Code. Prefer not to expose this path in Claude's environment.
- If testability is poor, factor startup preparation into a helper that accepts loaded config and returns `proxy.Config`.
- Avoid long-running tests that need real Claude Code.
- Use `httptest.Server` for upstream request inspection where possible.

**Measurable indicators of success:**

- `go test ./...` passes.
- No test requires Claude Code to be installed.
- No prompt contents appear in environment variables captured by launcher tests.
- Existing launcher tests pass without adding prompt-related env vars.

**Suggested tests:**

- `TestLauncherDoesNotExposeSystemPromptEnv`
- Helper-level main package test that valid prompt file content reaches `proxy.Config`.
- Helper-level main package test that invalid prompt file returns an error before server construction.

**Context budget guidance:** This task should fit in one 50k token context by loading `cmd/detour`, `internal/launcher`, and relevant config/proxy helper signatures only.

---

### Task 7: Documentation and examples

**Objective:** Update user-facing documentation so users can discover and safely use the new flag.

**Primary files:**

- `README.md`
- Optional: new example prompt under `docs/` if maintainers want a sample.

**Implementation notes:**

- Add `--system-prompt-file` to usage examples.
- Document that the file is read once at startup.
- Document merge behavior with existing request `system` fields.
- Warn users that prompt contents will be sent to whichever upstream handles each `/v1/messages` request.
- Mention size and UTF-8 requirements.
- Do not include a large prompt example in the README.

**Measurable indicators of success:**

- README contains the new flag name.
- README states the file is optional and read once at startup.
- README states the prompt is sent upstream in message requests.
- Documentation does not claim prompt contents are persisted.

**Suggested checks:**

- `rg "system-prompt-file|System prompt" README.md docs`
- Manual review of examples for copy/paste correctness.

**Context budget guidance:** This task should fit in one 50k token context by loading only `README.md` and this plan.

---

### Task 8: Final verification and release readiness

**Objective:** Confirm implementation quality, compatibility, and operational safety before merging.

**Primary files:**

- All changed files.

**Implementation notes:**

- Run all Go tests.
- Run formatting.
- Inspect persisted config file tests carefully to ensure prompt contents are not saved.
- Inspect error paths to ensure prompt contents are never logged.
- Test a manual local flow if the repository has an existing harness or mock LLM.

**Measurable indicators of success:**

- `gofmt` produces no diff.
- `go test ./...` passes.
- Manual or automated request inspection shows injected prompt in `/v1/messages` only.
- No prompt contents appear in logs, env vars, or persisted config.
- Feature is backward compatible when `--system-prompt-file` is omitted.

**Suggested commands:**

```bash
gofmt -w cmd/detour internal/config internal/proxy internal/launcher
go test ./...
rg "SystemPrompt|system-prompt-file|system_prompt" .
```

**Context budget guidance:** This task should fit in one 50k token context if the agent loads only failing test output and direct changed files.

## Cross-Task Acceptance Criteria

The feature is complete only when all of the following are true:

- `--system-prompt-file` is accepted by the CLI and documented.
- Omitting the flag preserves existing behavior.
- A valid UTF-8 prompt file is read once at startup.
- Invalid prompt files fail fast before proxy startup.
- `/v1/messages` requests receive the injected prompt according to the merge policy.
- Other routes are not modified by this feature.
- Prompt contents are not persisted, logged, or added to the Claude launcher environment.
- The full test suite passes.

## Open Questions for Maintainers

These are not blockers for the default plan, but they should be resolved before implementation if maintainers disagree with the recommended defaults.

1. Should injection apply to passthrough requests sent to Anthropic, or only to local-model requests? This plan recommends both for route-independent behavior.
2. Should a CLI-provided empty value be able to clear a saved `system_prompt_file` path? Existing config merge behavior may not support distinguishing omitted flags from empty flags.
3. Should prompt injection prepend or append relative to existing system content? This plan recommends prepending.
4. Should the feature support multiple prompt files later? This plan intentionally supports only one file.
5. Should the prompt file path be persisted in `~/.detour/config.json`? This plan allows path persistence but forbids content persistence.
