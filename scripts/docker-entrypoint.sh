#!/bin/sh
# Docker entrypoint for detour + Claude Code container.
# Translates environment variables into detour flags and handles the
# --no-claude proxy-only mode used by smoke tests.

set -eu

# Set HOME from /etc/passwd for the current UID so claude-code finds its config
# regardless of whether we run as root (UID 0) or the detour user (UID 1000).
if [ -z "${HOME:-}" ]; then
  _uid=$(id -u)
  HOME=$(awk -F: -v uid="$_uid" '$3 == uid { print $6; exit }' /etc/passwd 2>/dev/null)
  HOME=${HOME:-/root}
  export HOME
fi

# Configure Claude Code for musl-based Alpine
# ripgrep is installed separately; disable builtin ripgrep
export USE_BUILTIN_RIPGREP=0

# Generate per-launch auth token if not provided
if [ -z "${ANTHROPIC_DETOUR_AUTH:-}" ]; then
  ANTHROPIC_DETOUR_AUTH="$(head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32)"
  export ANTHROPIC_DETOUR_AUTH
fi

# Build detour args from env vars (empty values omit the flag)
# Also parse command-line args for --model-name, --model-api, --port if env vars not set
DETOUR_ARGS=""
: "${DETOUR_MODEL_NAME:=""}"
: "${DETOUR_MODEL_API:=""}"
: "${DETOUR_PORT:=""}"

[ -n "$DETOUR_MODEL_NAME" ] && DETOUR_ARGS="${DETOUR_ARGS} --model-name $DETOUR_MODEL_NAME"
[ -n "$DETOUR_MODEL_API" ] && DETOUR_ARGS="${DETOUR_ARGS} --model-api $DETOUR_MODEL_API"
[ -n "$DETOUR_PORT" ] && DETOUR_ARGS="${DETOUR_ARGS} --port $DETOUR_PORT"

# Parse command-line args for detour flags if env vars not set
while [ $# -gt 0 ]; do
  case "$1" in
    --model-name)
      [ -z "$DETOUR_MODEL_NAME" ] && DETOUR_ARGS="${DETOUR_ARGS} --model-name $2"
      shift 2
      ;;
    --model-api)
      [ -z "$DETOUR_MODEL_API" ] && DETOUR_ARGS="${DETOUR_ARGS} --model-api $2"
      shift 2
      ;;
    --port)
      [ -z "$DETOUR_PORT" ] && DETOUR_ARGS="${DETOUR_ARGS} --port $2"
      shift 2
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

# Check for --no-claude flag (proxy-only mode, used by smoke tests)
NO_CLAUDE=0
if [ "${1:-}" = "--no-claude" ]; then
  NO_CLAUDE=1
  shift
fi

# Strip a leading -- separator (conventional in docker run for clarity:
# docker run image -- --some-flag)
if [ "${1:-}" = "--" ]; then
  shift
fi

# If _DRY_RUN is set, echo the final command and exit (for unit testing)
if [ "${_DRY_RUN:-}" = "1" ]; then
  if [ "$NO_CLAUDE" = "1" ]; then
    echo "detour${DETOUR_ARGS} -- $*"
  else
    echo "detour${DETOUR_ARGS} -- claude -- $*"
  fi
  exit 0
fi

# In proxy-only mode, run sleep infinity as the subprocess so the proxy
# stays alive without launching claude (used by smoke tests).
# In normal mode, pass remaining args to detour, which runs claude.
if [ "$NO_CLAUDE" = "1" ]; then
  exec tini -- detour${DETOUR_ARGS} -- sleep infinity
else
  exec tini -- detour${DETOUR_ARGS} -- "$@"
fi
