# Docker Container Plan

Status: planning · Owner: TBD · Last updated: 2026-05-04

## 1. Goals

Ship an isolated, batteries-included container that:

1. Builds on **Alpine Linux** and stays as small as possible (multi-stage build, static `detour` binary, only the runtime tools Claude Code actually needs).
2. **Drops the user directly into Claude Code** on `docker run`, with `detour` already in front of it routing traffic.
3. Supports **shell tool calls natively**. Claude Code's `Bash`, `Read`, `Write`, `Edit`, and `WebFetch` tools require a working `bash`/POSIX userland, `git`, `curl`, and CA certs — those must be installed in the runtime layer.
4. Works **with or without an Anthropic credential**:
   - With `ANTHROPIC_API_KEY` exported → Claude Code reuses it via the existing detour env injection.
   - With a mounted `~/.claude` config directory → OAuth tokens persist across runs.
   - With neither → the container drops the user at an interactive Claude Code login prompt.
5. Forwards **arbitrary `claude` flags** (notably `--dangerously-skip-permissions`) supplied after the image name on the `docker run` command line.
6. Updates the public docs (`README.md`, `docs/docker.md`, `CLAUDE.MD`) so users can discover and use the image without reading source.

## 2. Non-goals

- Bundling a local LLM inside the image (Ollama, vLLM, llama.cpp). The container still needs `--model-api` to point at an inference server reachable from the container network.
- Building a multi-arch GHCR pipeline beyond `linux/amd64` and `linux/arm64`.
- Replacing the existing `cmd/mockllm` test harness — the container reuses it.

## 3. Architecture

```
┌──────────────────── docker run ─────────────────────┐
│                                                       │
│   ENTRYPOINT  /usr/local/bin/detour-entrypoint.sh    │
│        │                                              │
│        ▼                                              │
│   detour --model-name $DETOUR_MODEL_NAME              │
│          --model-api  $DETOUR_MODEL_API               │
│          -- "$@"        ◄── claude flags from CMD     │
│        │                                              │
│        ├── starts loopback proxy on :$DETOUR_PORT     │
│        │                                              │
│        └── execs `claude` (Node CLI installed via npm)│
│                with ANTHROPIC_BASE_URL injected       │
│                                                       │
└───────────────────────────────────────────────────────┘
```

### Image layers

| Stage      | Base                     | Purpose                                                 |
|------------|--------------------------|---------------------------------------------------------|
| `builder`  | `golang:1.24-alpine`     | `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` of `detour`. |
| `runtime`  | `alpine:3.20`            | Minimal Alpine + Node.js + `claude` CLI + `detour` binary + entrypoint. |

`runtime` `apk add --no-cache` set:

| Package          | Why                                                      |
|------------------|----------------------------------------------------------|
| `nodejs`, `npm`  | Required to install and run `@anthropic-ai/claude-code`. |
| `bash`           | Claude Code's `Bash` tool defaults to bash semantics.    |
| `git`            | Claude Code uses `git` for status, diff, commits.        |
| `curl`           | Used by Claude Code's `WebFetch`/MCP probes and tools.   |
| `ca-certificates`| TLS to `api.anthropic.com` and HTTPS upstreams.          |
| `tini`           | PID 1 reaper so `Ctrl-C` kills both proxy and claude.    |

We deliberately skip bigger toolchains (Python, build-essential, etc.). Users who need them mount a project volume that already has the toolchain it depends on.

### Image size target

- **Hard cap:** 350 MB compressed (`docker image inspect <img> -f '{{.Size}}'`).
- **Stretch goal:** under 250 MB. Roughly: alpine (~8 MB) + nodejs+npm (~60 MB) + claude code package (~80 MB) + git+bash+curl (~30 MB) + detour static binary (~10 MB) ≈ ~190 MB.

## 4. Runtime contract

### Environment variables consumed by the entrypoint

| Variable                  | Default                | Purpose                                                                    |
|---------------------------|------------------------|----------------------------------------------------------------------------|
| `DETOUR_MODEL_NAME`       | *(empty)*              | Mapped to `--model-name`. If empty, detour starts in proxy-only mode and `claude` runs without local routing — equivalent to plain Claude Code. |
| `DETOUR_MODEL_API`        | *(empty)*              | Mapped to `--model-api`.                                                   |
| `DETOUR_PORT`             | `8888`                 | Mapped to `--port`.                                                        |
| `ANTHROPIC_API_KEY`       | *(empty)*              | Forwarded to `claude` if set.                                              |
| `ANTHROPIC_DETOUR_AUTH`   | auto-generated         | If unset the entrypoint generates a per-launch random token (see C2 fix).  |
| `CLAUDE_HOME`             | `/root/.claude`        | Where to expect/write Claude Code's config and OAuth tokens.               |
| `DETOUR_HOME`             | `/root/.detour`        | Where to write detour's saved config (`config.json`).                      |
| `DETOUR_LOG`              | unset                  | Pass-through; turns on detour request logging.                             |

### CLI contract

```
docker run [docker-flags] trydydd/detour[:tag] [-- claude-args...]
```

- Everything after the image name is treated as `claude` arguments, including `--dangerously-skip-permissions`.
- The literal `--` separator is supported but optional; the entrypoint always prepends `--` when calling `detour`, so positional flags never collide with detour's own flags.
- If the user passes `--no-claude` (a custom flag the entrypoint recognises) the entrypoint runs `detour` in proxy-only foreground mode — useful for CI smoke tests and when wanting to attach Claude Code from outside the container.

### Auth modes

| Mode                  | Invocation                                                                                          | Behaviour                                                      |
|-----------------------|------------------------------------------------------------------------------------------------------|----------------------------------------------------------------|
| **Anthropic API key** | `docker run -e ANTHROPIC_API_KEY=… …`                                                               | Key flows through to `claude`, no interactive login.           |
| **Persistent OAuth**  | `docker run -v claude-config:/root/.claude …` (run `claude login` the first time)                   | OAuth refresh tokens persist across container restarts.        |
| **Ephemeral OAuth**   | `docker run -it … claude` then `claude login` inside                                                 | Login completes for the life of the container.                 |
| **No auth**           | `docker run -it …` and skip login                                                                    | Claude Code prompts for login on first request.                |

Detour itself never sees the user's Anthropic credentials — it only forwards the headers it already trusts (`Authorization`, `X-Api-Key`). The container does not change that boundary.

## 5. File layout

New files:

```
Dockerfile                        # multi-stage build
.dockerignore                     # excludes bin/, dist/, .git, etc.
scripts/docker-entrypoint.sh      # runtime launcher
scripts/test-docker.sh            # build + smoke tests for CI
docs/docker.md                    # user-facing guide
work-ledger-docker.yaml           # RALPH-friendly task list (this work)
docs/docker-container-plan.md     # this document
```

Modified files:

```
README.md                         # add "Run with Docker" section
CLAUDE.MD                         # note that the project ships a container image
Makefile                          # add image / image-test / image-publish targets
.github/workflows/ci.yml          # add docker build + smoke step
.github/workflows/release.yml     # publish image alongside binaries
```

## 6. Entrypoint design

`scripts/docker-entrypoint.sh` (sketch — final version lives in the ledger task D2):

```sh
#!/bin/sh
set -eu

# 1. Generate per-launch auth token if the caller didn't pass one.
if [ -z "${ANTHROPIC_DETOUR_AUTH:-}" ]; then
  ANTHROPIC_DETOUR_AUTH="$(head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32)"
  export ANTHROPIC_DETOUR_AUTH
fi

# 2. Build detour args from env vars. Empty values omit the flag.
set -- detour \
  ${DETOUR_MODEL_NAME:+--model-name "$DETOUR_MODEL_NAME"} \
  ${DETOUR_MODEL_API:+--model-api "$DETOUR_MODEL_API"} \
  ${DETOUR_PORT:+--port "$DETOUR_PORT"} \
  -- "$@"

exec tini -- "$@"
```

The entrypoint is intentionally tiny and POSIX-sh, not bash, so it can run before `bash` is on PATH if we ever go busybox-only.

## 7. Documentation updates

The plan ships with the following doc deltas (each captured as a ledger item):

- `README.md` § new "Run with Docker" — three copy-pasteable examples covering the three auth modes plus `--dangerously-skip-permissions`.
- `docs/docker.md` — long-form guide: build instructions, image variants, volume strategy, debugging, exit codes, image size ceiling, security notes (cleartext warning still applies).
- `CLAUDE.MD` — short note pointing future Claude Code sessions at `docs/docker.md` so they can answer container questions without re-reading source.

## 8. Testing strategy

| Layer        | Test                                                                                                            | Location                       |
|--------------|-----------------------------------------------------------------------------------------------------------------|--------------------------------|
| Build        | `docker build` succeeds on amd64 and arm64.                                                                      | `scripts/test-docker.sh`       |
| Size         | Final image ≤ 350 MB.                                                                                            | `scripts/test-docker.sh`       |
| Smoke        | `docker run --rm <img> --no-claude --help` exits 0 and prints detour usage.                                      | `scripts/test-docker.sh`       |
| Auth-pass    | With `ANTHROPIC_DETOUR_AUTH` unset, container generates a 32-char token and exposes it to the launched process.  | Go unit test in `cmd/detour`   |
| Integration  | Container + `cmd/mockllm` exchange a `/v1/messages` round-trip routed via the local alias.                       | `scripts/test-docker.sh`       |
| Pass-through | `docker run … -- --dangerously-skip-permissions --version` results in `claude` receiving those exact args.      | shell test using `--no-claude` shim — see ledger D8 |

CI runs the build + size + smoke tests on every PR. The integration test runs nightly.

## 9. Release / publish

- Image name: `ghcr.io/trydydd/detour`.
- Tags on release: `:vX.Y.Z`, `:latest`.
- Tags on push to `main`: `:edge`, `:sha-<short>`.
- `release.yml` reuses goreleaser-built binary if practical, otherwise builds inside the image (cost: a few extra seconds in the action).

## 10. Risks & mitigations

| Risk                                                                              | Mitigation                                                                                              |
|-----------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------|
| Claude Code Node CLI changes its install path or name.                            | Pin `@anthropic-ai/claude-code` to a tested minor version in the Dockerfile; bump explicitly.            |
| Alpine `musl` causes Node.js native module breakage.                              | The Claude Code CLI is pure JS; if a transitive native dep breaks, fall back to `node:22-alpine` final.  |
| Image grows past 350 MB.                                                          | CI size assertion fails the build; revisit the dependency list (drop `git`/`curl` only as last resort). |
| Per-launch auth token leaks via `ps`/`/proc/<pid>/environ` to other container PIDs.| Token is set in env, not argv; only one user/process runs in the container by design.                   |
| Cleartext-upstream warning fires inside container without obvious context.        | `docs/docker.md` repeats the H4 guidance and shows the SSH-tunnel pattern.                              |

## 11. Out-of-scope follow-ups

- Distroless or scratch base — would require statically linking Node.js, not worth the maintenance cost today.
- Rootless user inside the image — currently runs as `root` to keep volume-mount UX simple. A `--user` flag note in `docs/docker.md` covers users who care.
- Devcontainer / VS Code remote spec — once the base image is stable, generate a `.devcontainer/devcontainer.json` in a follow-up.
