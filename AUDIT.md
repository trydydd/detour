# Security Audit — detour

**Audit date:** 2026-05-02
**Scope:** Full source review of the detour HTTP proxy, with emphasis on outbound internet connections, credential handling, and supply chain risk.
**Methodology:** Manual code review of all `.go` files in the repository, dependency analysis of `go.mod`/`go.sum`, threat-modeling of network surfaces.

---

## Architecture summary

`detour` is a localhost HTTP proxy that sits between Claude Code (a CLI client) and two upstream services:

1. **Anthropic API** (`https://api.anthropic.com`) — for "passthrough" model traffic.
2. **A local inference server** (URL provided at runtime via `--model-api`) — for the user-aliased model (e.g. `red`).

The proxy forwards Claude Code's authenticated requests to whichever upstream matches the requested `model`. It also launches `claude` as a subprocess with `ANTHROPIC_BASE_URL` pointing at itself.

External network calls in the codebase:
- `forward.Do(...)` → outbound to Anthropic over HTTPS (forwards `Authorization` / `X-Api-Key`).
- `forward.DoLocal(...)` → outbound to user-supplied local URL, which may be plain HTTP across a LAN.

Supply chain surface:
- `go.mod` declares **zero third-party dependencies** — the project only imports the Go standard library. There is no `go.sum` because there are no external modules to pin.

---

## Findings

### 🔴 CRITICAL

#### C1 — Proxy listens on all network interfaces, not just loopback
**Location:** `cmd/detour/main.go:57` — `addr := fmt.Sprintf(":%d", cfg.Port)`

The proxy binds to `:8888`, which on Linux/macOS resolves to `0.0.0.0` (all interfaces). Combined with finding **C2** below, this means **any host on the same network can reach the proxy and send arbitrary requests to Anthropic that will be authenticated using the user's stored API credentials** (since Claude Code's auth headers are forwarded). Attackers on the same Wi-Fi, LAN, VPN, or container network can:

- Burn through the user's Anthropic API quota / spend money against their account.
- Issue arbitrary prompts and read responses.
- Use the proxy as an exfiltration channel for prompt injection attacks.

**Fix:** Bind explicitly to loopback:

```go
addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
```

If multi-host access is ever needed, gate it behind an explicit `--bind` flag with a default of `127.0.0.1`, and pair with auth (see C2).

---

#### C2 — No authentication on the proxy
**Location:** `internal/proxy/handler.go` — none of the handlers verify the caller.

Any process that can connect to the proxy port can send requests. The proxy then attaches whatever auth headers Claude Code (or the caller) sends and forwards them to Anthropic. There is no shared secret, no `localhost`-origin check, no PID/UID check, and no proof-of-Claude-Code.

This is "critical" only when combined with C1, but is independently relevant: any local user on a multi-user box can also abuse the proxy.

**Fix options (pick one):**
1. Generate a random per-launch secret token, set it in Claude Code's environment (`ANTHROPIC_AUTH_TOKEN` or a custom header), and require it on every inbound request. This is the cleanest mitigation.
2. After fixing C1, on multi-user systems, additionally check `r.RemoteAddr`'s peer credentials via Unix socket (would require switching from TCP to a Unix socket listener).

---

### 🟠 HIGH

#### H1 — No HTTP server timeouts → trivial slowloris / resource exhaustion
**Location:** `cmd/detour/main.go:58` — `srv := &http.Server{Addr: addr, Handler: mux}`

The `http.Server` is constructed without `ReadTimeout`, `ReadHeaderTimeout`, `WriteTimeout`, or `IdleTimeout`. A single malicious or buggy client can hold connections open indefinitely, send bytes one at a time, or open thousands of idle connections to exhaust file descriptors.

**Fix:**
```go
srv := &http.Server{
    Addr:              addr,
    Handler:           mux,
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       60 * time.Second,
    WriteTimeout:      0, // 0 because SSE streams may run for minutes
    IdleTimeout:       120 * time.Second,
}
```

`WriteTimeout` must stay at `0` (or be very large) because `/v1/messages` can stream for minutes. Consider per-handler timeouts via `http.TimeoutHandler` wrapping non-streaming endpoints if you want stricter limits.

---

#### H2 — Unbounded request body read
**Location:** `internal/proxy/handler.go:58` — `body, err := io.ReadAll(r.Body)`

`io.ReadAll` will read the entire request body into memory with no size cap. A client can send a 10GB body and OOM the proxy. Anthropic's actual upper bound is much smaller; we should enforce a sane cap before parsing.

**Fix:**
```go
const maxRequestBytes = 10 << 20 // 10 MiB
body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
if len(body) > maxRequestBytes {
    writeError(w, http.StatusRequestEntityTooLarge, "invalid_request", "body too large")
    return
}
```

---

#### H3 — `--model-api` URL is unvalidated and concatenated, allowing scheme/path manipulation
**Location:** `internal/proxy/handler.go:79`, `cmd/detour/main.go:53`

`cfg.ModelAPI` is taken from a CLI flag and persisted to `~/.detour/config.json` without any URL parsing or scheme validation. It is then concatenated directly: `cfg.LocalUpstreamURL + "/v1/messages"`.

Risks:
- A user (or a process that can write `~/.detour/config.json`) can set `--model-api` to `file:///etc/passwd` — `http.DefaultClient.Do` will reject `file://`, but other schemes like `https://attacker.example.com` are honored silently.
- A trailing slash mismatch, or a URL like `http://victim.example.com#`, could cause request smuggling on certain upstream behaviors.
- The persisted config has no integrity protection — anything with write access to `~/.detour/` can change the upstream silently on next run.

**Fix:**
1. Parse the URL with `url.Parse`, reject anything that isn't `http` or `https`, reject any path/query/fragment, and store only the canonicalized origin.
2. Refuse to load `config.json` if its mode is group/world-writable (`os.Stat` then check `Mode().Perm()`).
3. Write `config.json` with `0o600`, not the default umask.

---

#### H4 — Plain-HTTP traffic to the local inference server crosses the network in cleartext
**Location:** `cmd/detour/main.go:25` — flag default; `internal/forward/forward.go:96`

The documented usage is `--model-api http://192.168.0.214:8000`. When the local server runs on a different host (which is the documented case), every Claude Code request body — which can contain source code, secrets pasted into prompts, internal docs, etc. — is sent in cleartext over the LAN. Any host on the same broadcast domain can sniff it.

This is partly a deployment choice, but the tool currently does nothing to warn the user, and silently accepts `http://` URLs.

**Fix:**
- When `--model-api` is `http://` and the host is **not** loopback (`127.0.0.1`, `::1`, `localhost`), print a prominent warning at startup that traffic will be unencrypted.
- Document the recommended pattern: terminate TLS at the inference server, or run an SSH tunnel.

---

### 🟡 MEDIUM

#### M1 — Verbose upstream errors leak network/internal details to clients
**Location:** `internal/forward/forward.go:43,98` — `"upstream unavailable: " + err.Error()`

Returning raw Go network errors to the HTTP response can disclose internal IPs, DNS lookup state, or proxy configuration to the caller. With C1 unfixed, that caller may be remote.

**Fix:** Log the detailed error server-side (stderr) and return a generic message to the client:
```go
fmt.Fprintf(os.Stderr, "detour: upstream error: %v\n", err)
writeError(w, http.StatusBadGateway, "proxy_error", "upstream unavailable")
```

---

#### M2 — Config file written with default umask
**Location:** `internal/config/config.go:44` — `os.Create(...)` uses `0o666 & ~umask`

`config.json` may end up world-readable (e.g. `0644` on a typical box). It contains the local model API URL (likely an internal IP), which is low-sensitivity but unnecessary leakage on shared hosts.

**Fix:** Replace `os.Create` with `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)`. Likewise, `os.MkdirAll(dir, 0o755)` should be `0o700`.

---

#### M3 — No SSRF protection on the local upstream
**Location:** `internal/forward/forward.go:96`

Once `--model-api` is set, all `/v1/messages` requests for the aliased model go to that URL with no further validation. If the model URL is later changed by an attacker who has write access to `~/.detour/config.json` (see H3), they can redirect Claude Code's traffic to an attacker-controlled endpoint. Since `Authorization` is stripped for `DoLocal`, the direct credential leak is mitigated — but the **prompt content** still flows to the attacker, which often contains source code, internal data, and pasted secrets.

**Fix:** Combine H3's URL validation with a startup-time DNS resolution + IP allowlist check, and refuse to send traffic to URLs that resolve to public IPs unless the user passes a `--allow-public-upstream` opt-in.

---

#### M4 — `claude` binary resolved via `$PATH` with no integrity check
**Location:** `internal/launcher/launcher.go:17` — `claudeBin = "claude"`; `exec.Command(claudeBin, ...)`

`exec.Command("claude", ...)` searches `$PATH`. If a user's `$PATH` contains a writable directory (a stale `./node_modules/.bin`, a `~/bin` shadowed by a rogue symlink, etc.), an attacker who can drop a `claude` binary in that directory will get full code execution under the user when they run `detour`. This is a generic Unix concern but worth calling out because `detour` exists specifically to wrap `claude` and inherits its trust boundary.

**Fix:** Resolve `claude` once via `exec.LookPath`, log the resolved absolute path at startup so the user notices a surprise, and consider adding a `--claude-bin` flag for explicit pinning.

---

#### M5 — Streaming copy ignores write errors and request cancellation
**Location:** `internal/forward/forward.go:64-79`, `internal/forward/forward.go:175-209`

`copyStreaming` and `copyStreamingStripThinking` loop on `body.Read` without checking `r.Context().Done()` or the client's write errors. A disconnected client can leave the proxy holding open the upstream stream until it naturally terminates — wasting Anthropic API tokens and proxy goroutines.

**Fix:** Pass the request `context.Context` down, and break the loop on context cancellation or when `w.Write` returns an error.

---

### 🟢 LOW

#### L1 — Health endpoint advertises proxy version
**Location:** `internal/proxy/handler.go:48-54`

`/health` returns `{"status":"ok","version":"0.1.0"}`. Knowing the version lets an attacker target known bugs in older releases. Low risk because there's no public release history yet, but worth removing once versions ship.

**Fix:** Drop `version` from the health response, or gate it behind an authenticated debug endpoint.

---

#### L2 — `Authorization` header is forwarded to Anthropic but the comment says only `X-Api-Key`
**Location:** `internal/forward/forward.go:12-18`

`allowedHeaders` includes both `X-Api-Key` and `Authorization`. This is fine — both are valid Anthropic auth schemes — but the dual paths add a small footgun: a client that accidentally sets *both* with conflicting values gets undefined behavior. Document the intent.

**Fix:** Either strip one if both are present, or add a comment explaining that both are intentionally forwarded for compatibility with different Claude Code auth modes.

---

#### L3 — Anthropic-Beta filter is purely substring-based
**Location:** `internal/proxy/handler.go:116-125`

`filterThinkingBeta` drops any beta token containing the substring `"thinking"`. If Anthropic ever releases a beta whose name includes `"thinking"` for a non-thinking-block feature, it will be silently stripped on local-routed requests.

**Fix:** Match against an explicit allowlist/denylist of known thinking-related beta tokens (e.g. `interleaved-thinking-2025-05-14`, `dev-full-thinking-2025-05-14`).

---

#### L4 — Subprocess inherits the entire parent environment
**Location:** `internal/launcher/launcher.go:19,30`

`buildEnv` starts from `os.Environ()` and only overrides specific keys. `claude` therefore sees every environment variable the user had — including anything sensitive in shell startup files. This is normal Unix behavior, not a vulnerability, but worth noting in the threat model.

**Fix:** Optional. Could allowlist a small set of variables (`HOME`, `PATH`, `USER`, `LANG`, `TERM`, `ANTHROPIC_*`) and drop the rest. Likely too restrictive in practice.

---

#### L5 — Body is buffered then re-wrapped with `io.NopCloser`
**Location:** `internal/proxy/handler.go:58, 89`

Reading the request fully into memory and then handing a fresh reader to `forward.Do` means slow upload streaming is impossible. Combined with H2, this is fine once a size cap exists; without it, an attacker can pin memory.

**Fix:** Already covered by H2 (cap the read).

---

### ✅ POSITIVE FINDINGS

- **Zero third-party Go dependencies.** Supply-chain attack surface is restricted to the Go toolchain itself. This is excellent and should be preserved — every new dependency multiplies risk.
- **Anthropic upstream is hardcoded as `https://api.anthropic.com`** at `cmd/detour/main.go:54`. There is no flag to override it, so a config-file attacker cannot redirect the trusted upstream.
- **`Authorization` header is stripped before forwarding to the local inference server** (`forward.allowedLocalHeaders`). The local server, even if hostile, never sees the user's Anthropic credentials.
- **Thinking blocks are stripped from local responses** to prevent forged-signature blocks from poisoning subsequent passthrough requests. This is a thoughtful defense.
- **Default Go TLS validation is in effect** for the Anthropic call — no `InsecureSkipVerify`, no custom `Transport`, no overridden CA pool. The system trust store is honored.
- **Request contexts are propagated** to outbound requests via `http.NewRequestWithContext`, so client cancellation correctly aborts the upstream call (subject to M5).

---

## Suggested remediation order

1. **C1 + C2 together.** Bind to `127.0.0.1` and add a per-launch shared-secret header. Without these, every other finding is academic — the whole proxy is a remote-accessible credential bridge.
2. **H1 + H2.** Server timeouts and a body size cap. Cheap and prevents trivial denial-of-service.
3. **H3 + H4.** Validate `--model-api`, tighten config file permissions, warn on cleartext non-loopback upstreams.
4. **M1.** Stop returning raw Go errors to clients.
5. **M2 – M5 and the LOW findings** as time permits.

---

## Supply chain notes

- `go.mod` declares Go 1.24 with no `require` block. `go.sum` does not exist. There is no risk from compromised third-party modules in this repository's build.
- The `go install github.com/trydydd/detour/cmd/detour@main` workflow used during testing fetches code from GitHub and builds it with the user's local Go toolchain. The integrity guarantees are whatever GitHub + Go's checksum database (`sum.golang.org`) provide. Recommend pinning to tagged releases (`@v0.1.0`) once the project starts cutting them.
- The launcher's reliance on `$PATH` to find the `claude` binary (M4) is the largest single supply-chain-adjacent risk in the running system.
