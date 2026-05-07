# Implementation Plan

This plan tracks the next actionable work for the **detour** project. It is the loop's
canonical task source. The two ledgers (`work-ledger.yaml`, `work-ledger-docker.yaml`)
remain the source of truth for acceptance criteria and TDD steps — this file just
selects the next slice and orders it by priority.

## Current State (verified 2026-05-05)

- **Security ledger** (`work-ledger.yaml`): C1, C2, H2, H3, H4 completed. H1 and the M/L tiers are pending.
- **Docker ledger** (`work-ledger-docker.yaml`): D1–D11 marked completed; D12 pending. However, a code review of D2 / D7 / D8 surfaces real bugs in those "completed" items — captured below as D2-fix, D7-fix, and D8-fix.
- Codebase confirms: `cmd/detour/main.go:62` constructs `&http.Server{Addr: addr, Handler: mux}` with no timeouts (H1 still real). `internal/proxy/handler.go:53` still emits `version` from `/health` (L1 still real). `internal/config/config.go:94` already writes config with `0o600` (M2 already done — ledger is stale).

## High Priority

- [ ] **D2-fix — Drop the redundant `claude --` literal from the docker entrypoint**
  - Files to modify: `scripts/docker-entrypoint.sh`, `scripts/test-entrypoint.sh`
  - Bug: `scripts/docker-entrypoint.sh:49` exec's `detour ${DETOUR_ARGS} -- claude -- "$@"`. But `cmd/detour/main.go:79` already runs `claude` itself via `launcher.Launch` using `flag.Args()` (`cmd/detour/main.go:28`). The literal `claude --` therefore ends up *inside* `flag.Args()` — i.e. the launched command becomes `claude claude -- "$@"`, which silently breaks every container invocation that actually relies on argv pass-through.
  - Fix: collapse the two branches in `docker-entrypoint.sh` to `exec tini -- detour${DETOUR_ARGS} -- "$@"`. The `--no-claude` flag should still be honored (it is the marker our smoke test uses), but the resulting argv shape is the same — detour stops launching claude on its own when this flag is set, but the entrypoint shouldn't.
  - Verify: `_DRY_RUN=1 scripts/docker-entrypoint.sh --version` → expect `detour --  --version` (or similar without "claude --"). Update `scripts/test-entrypoint.sh` expected strings accordingly: "No model env vars" → `detour -- ` (not `detour -- claude -- `), and the model-vars case → `detour --model-name red --model-api http://x -- ` (not `... -- claude -- `).
  - Re-open the relevant ledger item in `work-ledger-docker.yaml` (D2) when starting; mark it back to `requires_validation` after.

- [ ] **D8-fix — Make `scripts/test-claude-args.sh` actually verify argv pass-through**
  - Files to modify: `scripts/test-claude-args.sh`
  - Bug: the current script builds a derived image with a `claude` shim that writes `"$@"` to `/tmp/claude-argv`, but it never reads that file back. Instead it runs `docker run --entrypoint sh "$TEST_IMAGE" -c "claude --dangerously-skip-permissions --version"` (which doesn't exercise the entrypoint at all), creates an unrelated `CONTAINER_ID` from `sh -c 'sleep 30'`, then reports `pass "Test framework in place"`. The script's own comments admit "this approach won't work easily". D8's acceptance criteria are unmet.
  - Fix (sequence with D2-fix above):
    1. Rewrite the harness to drive the real entrypoint: `docker run --rm "$TEST_IMAGE" --no-claude --version` is wrong here — we *want* claude to be invoked. Instead, run the container without `--no-claude` and let the shimmed `claude` exit 0 after writing argv to a path that survives the run (e.g. via a bind-mounted volume).
    2. After the run, read the captured argv from the host-side mounted file and assert exact equality with `--dangerously-skip-permissions --version`.
    3. Add a second case asserting the empty argv path: `docker run --rm "$TEST_IMAGE"` produces an empty argv on the claude side.
    4. Remove the unconditional `pass "Test framework in place"`.

- [ ] **H1 — Add HTTP server timeouts to prevent slowloris**
  - Files to modify: `cmd/detour/main.go`, `cmd/detour/main_test.go`
  - Implementation notes:
    - Replace the bare `&http.Server{Addr: addr, Handler: mux}` at `cmd/detour/main.go:62` with timeouts: `ReadHeaderTimeout: 10s`, `ReadTimeout: 60s`, `WriteTimeout: 0` (preserve SSE), `IdleTimeout: 120s`.
    - TDD: add a unit test that constructs the server via a small helper (e.g. `newServer(addr, mux)`) and asserts the four timeout fields. Refactor `main` to use the helper so it is testable without `go func()` plumbing.
    - Acceptance criteria from ledger H1.

- [ ] **M1 — Sanitize upstream errors returned to client**
  - Files to modify: `internal/forward/forward.go`, `internal/forward/forward_test.go`
  - Implementation notes:
    - At `forward.go:43` and `forward.go:98` (raw error sites flagged in audit), log the detailed `err` to stderr via `fmt.Fprintf(os.Stderr, "detour: upstream error: %v\n", err)` and return a generic `writeError(w, http.StatusBadGateway, "proxy_error", "upstream unavailable")` to the client.
    - TDD: add a forward test that injects a transport returning a network error and asserts the response body contains only the generic message (no IP, hostname, or `dial tcp` text).
    - Acceptance criteria from ledger M1.

- [ ] **M2 — Mark ledger entry as completed (no code change required)**
  - Files to modify: `work-ledger.yaml` only
  - Implementation notes:
    - Verified at `internal/config/config.go:94` — `os.OpenFile(..., 0o600)` already in place, with permission read-back check at line 112. Flip `status: pending` → `status: completed` and add a `fix_notes` line pointing to the existing implementation.
    - No tests required; existing `internal/config/config_test.go` covers the behavior.

## Medium Priority

- [ ] **D7-fix — Emit `:edge` and `:sha-<short>` tags for main-branch image pushes**
  - Files to modify: `.github/workflows/release.yml`
  - Gap: ledger D7 acceptance requires `:vX.Y.Z` + `:latest` on tag pushes, and `:edge` + `:sha-<short>` on default-branch pushes. The current workflow at `.github/workflows/release.yml:60-66` always emits `:${{ steps.vars.outputs.tag }}` plus `:latest`. On a main-branch push that becomes `:edge` + `:latest`, which (a) is incorrect — `:latest` should track tagged releases — and (b) still misses `:sha-${short}`.
  - Fix: branch the `tags:` block on `github.ref_type`. Tag push: `:${tag}` and `:latest`. Branch push: `:edge` and `:sha-${short}` (the existing `steps.vars.outputs.short_sha` is already computed). Keep the multi-arch + buildx caching unchanged.

- [ ] **D5-fix — De-flake the mockllm integration test**
  - Files to modify: `scripts/test-docker.sh`
  - Issues at `scripts/test-docker.sh:106-152`:
    - Hardcoded `CONTAINER_PORT=8888` collides with anything else bound on the host (the test uses `--network=host`).
    - Two blind `sleep 2` calls instead of readiness probes — racy on busy CI runners.
    - `AUTH_TOKEN="${CONTAINER_AUTH:-test-auth-token}"` does not match the random per-launch token the entrypoint generates. The test currently appears to pass only because the proxy accepts unknown tokens when its env is unset — that's coincidence, not contract.
  - Fix:
    - Pick a free proxy port via a Go helper or `python -c 'import socket; …'`, pass it as `-e DETOUR_PORT=$port`.
    - Pass `-e ANTHROPIC_DETOUR_AUTH=$known_token` and use `$known_token` in the curl `x-api-key` header.
    - Replace each `sleep 2` with a tight `until curl -fsS http://127.0.0.1:$port/health; do sleep 0.1; done` loop with a 10s deadline.

- [ ] **CI-shellcheck — Add `shellcheck` to the existing CI job**
  - Files to modify: `.github/workflows/ci.yml`
  - Reason: D2's acceptance criteria explicitly require `shellcheck -s sh scripts/docker-entrypoint.sh` to pass, but CI never runs it. Add a step in the existing `test` job: `sudo apt-get install -y shellcheck && shellcheck -s sh scripts/docker-entrypoint.sh && shellcheck scripts/test-entrypoint.sh scripts/test-docker.sh scripts/test-claude-args.sh`.
  - Fix any findings; expect the `test-claude-args.sh` rewrite (D8-fix) to land first since shellcheck will probably flag the current script's many `|| true` + `set -eu` interactions.

- [ ] **M3 — SSRF protection on the local upstream**
  - Files to modify: `internal/config/config.go`, `cmd/detour/main.go`, `internal/config/config_test.go`
  - Implementation notes:
    - At config validation, resolve the upstream host via `net.LookupIP` and reject if any resolved address is not in the loopback or RFC1918 private ranges. Add a `--allow-public-upstream` boolean flag (default false) that bypasses the check.
    - Be careful with the existing `validateModelAPI` (`config.go:44`) — keep URL parsing separate from DNS resolution so the unit tests stay hermetic. Put the DNS check behind an injectable resolver.
    - Acceptance criteria from ledger M3.

- [ ] **M4 — Resolve `claude` binary at startup with `exec.LookPath`**
  - Files to modify: `internal/launcher/launcher.go`, `internal/launcher/launcher_test.go`, `cmd/detour/main.go`
  - Implementation notes:
    - At `launcher.go:17`, replace the implicit `$PATH` lookup with explicit `exec.LookPath("claude")` (or honor a new `--claude-bin` flag). Log the resolved absolute path to stderr so users see what is actually being executed.
    - TDD: stub `LookPath` via an injectable function variable; assert the resolved path is logged and that `--claude-bin` overrides discovery.
    - Acceptance criteria from ledger M4.

- [ ] **M5 — Honor request cancellation in streaming forwarder**
  - Files to modify: `internal/forward/forward.go`, `internal/forward/forward_test.go`
  - Implementation notes:
    - The streaming copy loops at `forward.go:64-79` and `forward.go:175-209` ignore `r.Context().Done()` and `w.Write` errors. Add `select { case <-ctx.Done(): return ... default: }` (or equivalent) inside each iteration and break on write errors.
    - TDD: drive a fake upstream that streams indefinitely, cancel the request context mid-stream, and assert the loop exits within a short deadline. Add a second test that simulates `w.Write` returning `errors.New(...)` and asserts the loop terminates.
    - Acceptance criteria from ledger M5.

## Low Priority

- [ ] **L1 — Drop version from `/health` response**
  - Files to modify: `internal/proxy/handler.go`, `internal/proxy/handler_test.go`
  - Implementation notes:
    - At `handler.go:14` and `handler.go:53`, remove the `version` constant and the `"version": version` map entry. Health response should be `{"status":"ok"}` only. Update or remove `internal/proxy/handler_test.go` assertions tied to the version field.

- [ ] **L2 — Document dual auth header forwarding**
  - Files to modify: `internal/forward/forward.go`
  - Implementation notes:
    - At `forward.go:12-18` (`allowedHeaders`), add a one-line comment explaining that both `X-Api-Key` and `Authorization` are forwarded intentionally for Claude Code compatibility. No behavior change.

- [ ] **L3 — Replace substring filter for thinking betas with explicit denylist**
  - Files to modify: `internal/proxy/handler.go`, `internal/proxy/handler_test.go`
  - Implementation notes:
    - At `handler.go:116-125`, replace `strings.Contains(token, "thinking")` with a lookup against a small `var thinkingBetas = map[string]struct{}{...}` containing the currently-known tokens (`interleaved-thinking-2025-05-14`, `extended-thinking-2025-05-14`, etc. — confirm exact list against current Anthropic docs).
    - TDD: add a hypothetical-token test (`some-thinking-feature-2026`) that verifies it now passes through.

- [ ] **L4 — Document subprocess env inheritance behavior**
  - Files to modify: `docs/docker.md` and/or `README.md`
  - Implementation notes:
    - Add a short threat-model note acknowledging `os.Environ()` inheritance at `internal/launcher/launcher.go:19,30`. Implementing an allowlist is optional and out of scope for this iteration — just document.

- [ ] **D12 — Run Docker image as non-root user (optional)**
  - Files to modify: `Dockerfile`, `docs/docker.md`, `scripts/test-docker.sh`, `scripts/test-claude-args.sh`
  - Implementation notes:
    - Add `RUNTIME_USER` build arg defaulting to `detour`, create UID/GID 1000, switch via `USER`, and adjust home paths. Update test scripts to handle the volume-mount permission change. Document the requirement to `chown 1000:1000` host-mounted volumes.
    - Acceptance criteria from ledger D12.

## Notes

- Tasks are atomic — each loop iteration should pick the topmost unchecked item, follow the TDD steps in the corresponding ledger entry, run validation, and commit.
- Skip an item only if its preconditions cannot be met in the current environment (e.g. Docker tasks require a Docker daemon).
- When an item is completed, also update its `status` field in the corresponding ledger so the two views stay in sync.
- L5 from the security ledger is intentionally omitted — it is fully covered by the already-completed H2 fix.
