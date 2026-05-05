#!/bin/sh
# Docker entrypoint for detour + Claude Code container
# This script:
#   - Generates ANTHROPIC_DETOUR_AUTH if unset (32 chars from /dev/urandom)
#   - Translates DETOUR_MODEL_NAME / DETOUR_MODEL_API / DETOUR_PORT env vars into detour flags
#   - Recognises --no-claude flag and runs detour in proxy-only mode
#   - Forwards remaining args to claude via detour
#   - Execs through tini for proper signal handling

set -eu

# Generate per-launch auth token if not provided
if [ -z "${ANTHROPIC_DETOUR_AUTH:-}" ]; then
  ANTHROPIC_DETOUR_AUTH="$(head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32)"
  export ANTHROPIC_DETOUR_AUTH
fi

# Build detour args from env vars (empty values omit the flag)
DETOUR_ARGS=""
DETOUR_ARGS="${DETOUR_ARGS}${DETOUR_MODEL_NAME:+ --model-name $DETOUR_MODEL_NAME}"
DETOUR_ARGS="${DETOUR_ARGS}${DETOUR_MODEL_API:+ --model-api $DETOUR_MODEL_API}"
DETOUR_ARGS="${DETOUR_ARGS}${DETOUR_PORT:+ --port $DETOUR_PORT}"

# Check for --no-claude flag (used for smoke tests)
NO_CLAUDE=0
if [ "${1:-}" = "--no-claude" ]; then
  NO_CLAUDE=1
  shift
fi

# If _DRY_RUN is set, echo the final command and exit (for testing)
if [ "${_DRY_RUN:-}" = "1" ]; then
  if [ "$NO_CLAUDE" = "1" ]; then
    echo "detour${DETOUR_ARGS} -- $*"
  else
    echo "detour${DETOUR_ARGS} -- claude -- $*"
  fi
  exit 0
fi

# Run detour (proxy-only mode if --no-claude)
if [ "$NO_CLAUDE" = "1" ]; then
  exec tini -- detour"${DETOUR_ARGS}" -- "$@"
else
  exec tini -- detour"${DETOUR_ARGS}" -- claude -- "$@"
fi
