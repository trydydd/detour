#!/bin/bash
# Docker smoke test suite.
# Runs on CI and local machines to validate the detour container image.
#
# Exit codes:
#   0 = all checks passed
#   1 = one or more checks failed

set -eu

IMAGE="${IMAGE:-detour:test}"
SIZE_LIMIT=367001600  # 350 MiB in bytes

PASS=0
FAIL=0
MOCK_PID=""
CONTAINER_NAME=""

if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  NC='\033[0m'
else
  RED=''
  GREEN=''
  NC=''
fi

pass() {
  echo "${GREEN}PASS${NC}: $1"
  PASS=$((PASS + 1))
}

fail() {
  echo "${RED}FAIL${NC}: $1"
  FAIL=$((FAIL + 1))
}

cleanup() {
  if [ -n "${MOCK_PID:-}" ] && kill -0 "$MOCK_PID" 2>/dev/null; then
    kill "$MOCK_PID" 2>/dev/null || true
    wait "$MOCK_PID" 2>/dev/null || true
  fi
  if [ -n "${CONTAINER_NAME:-}" ]; then
    docker stop "$CONTAINER_NAME" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "=== Docker Smoke Test Suite ==="
echo "Image: $IMAGE"
echo ""

# Check 1: Build image (unless skipped)
echo "--- Check 1: Build image ---"
if [ "${SKIP_BUILD:-0}" = "1" ]; then
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    pass "Skipping build (SKIP_BUILD=1) - image exists"
  else
    fail "SKIP_BUILD=1 but image '$IMAGE' not found"
    exit 1
  fi
else
  if docker build -t "$IMAGE" . 2>&1; then
    pass "Image built successfully"
  else
    fail "Image build failed"
    exit 1
  fi
fi

# Check 2: Image size assertion
echo ""
echo "--- Check 2: Image size (≤350 MiB) ---"
SIZE=$(docker image inspect "$IMAGE" --format '{{.Size}}' 2>/dev/null || echo "0")
if [ "$SIZE" -le "$SIZE_LIMIT" ] 2>/dev/null; then
  SIZE_MB=$((SIZE / 1024 / 1024))
  pass "Image size is ${SIZE_MB}MB (limit: 350MB)"
else
  SIZE_MB=$((SIZE / 1024 / 1024))
  fail "Image size is ${SIZE_MB}MB (limit: 350MB)"
fi

# Check 3: Container --no-claude --help output
echo ""
echo "--- Check 3: Container --no-claude --help ---"
HELP_OUTPUT=$(docker run --rm "$IMAGE" --no-claude --help 2>&1 || true)
if echo "$HELP_OUTPUT" | grep -q '\-\-model-name'; then
  pass "--help output contains --model-name flag"
else
  fail "--help output missing --model-name flag"
  echo "  Output preview: $(echo "$HELP_OUTPUT" | head -c 200)"
fi

# Check 4: Container --no-claude --version output
echo ""
echo "--- Check 4: Container --no-claude --version ---"
VERSION_OUTPUT=$(docker run --rm "$IMAGE" --no-claude --version 2>&1 || true)
if [ -n "$VERSION_OUTPUT" ]; then
  pass "--version output is non-empty: $(echo "$VERSION_OUTPUT" | head -c 50)"
else
  fail "--version output is empty"
fi

# Check 5: Integration test - mockllm round-trip
echo ""
echo "--- Check 5: Integration test (mockllm round-trip) ---"

MOCK_PORT=$((RANDOM % 10000 + 20000))
CONTAINER_NAME="detour-smoke-$$"

# Start mockllm on a random port bound to 127.0.0.1
go run ./cmd/mockllm --port "$MOCK_PORT" >/dev/null 2>&1 &
MOCK_PID=$!

# Wait for mockllm to be ready (go run compiles first, allow up to 30s)
MOCK_READY=0
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  curl -s "http://127.0.0.1:$MOCK_PORT/" >/dev/null 2>&1 && { MOCK_READY=1; break; }
  sleep 2
done

if [ "$MOCK_READY" = "0" ]; then
  fail "Mockllm did not start in time"
else
  # Run container in proxy-only mode: --no-claude causes the entrypoint to run
  # sleep infinity as the detour subprocess, keeping the proxy alive for testing.
  docker run -d --rm \
    --name "$CONTAINER_NAME" \
    --network=host \
    -e DETOUR_MODEL_NAME=detour-mock \
    -e DETOUR_MODEL_API=http://127.0.0.1:$MOCK_PORT \
    "$IMAGE" --no-claude >/dev/null 2>&1

  # Wait for detour proxy to be ready (checks /health endpoint)
  PROXY_READY=0
  for i in 1 2 3 4 5 6 7 8 9 10; do
    curl -s "http://127.0.0.1:8888/health" >/dev/null 2>&1 && { PROXY_READY=1; break; }
    sleep 1
  done

  if [ "$PROXY_READY" = "0" ]; then
    fail "Detour proxy did not start in time"
  else
    RESPONSE=$(curl -s -X POST "http://127.0.0.1:8888/v1/messages" \
      -H "Content-Type: application/json" \
      -H "x-api-key: test-token" \
      -d '{"model":"detour-mock","messages":[{"role":"user","content":"test"}]}' 2>/dev/null || true)

    if echo "$RESPONSE" | grep -q 'THIS IS DETOUR TEST!'; then
      pass "Mockllm round-trip successful - response contains sentinel"
    else
      fail "Mockllm round-trip failed - response missing sentinel"
      echo "  Response preview: $(echo "$RESPONSE" | head -c 200)"
    fi
  fi

  docker stop "$CONTAINER_NAME" 2>/dev/null || true
  CONTAINER_NAME=""
  kill "$MOCK_PID" 2>/dev/null || true
  wait "$MOCK_PID" 2>/dev/null || true
  MOCK_PID=""
fi

# Check 6: Claude args pass-through test
echo ""
echo "--- Check 6: Claude args pass-through ---"
SCRIPTDIR="$(cd "$(dirname "$0")" && pwd)"
if bash "$SCRIPTDIR/test-claude-args.sh"; then
  pass "Claude args pass-through test passed"
else
  fail "Claude args pass-through test failed"
fi

# Summary
echo ""
echo "=== Test Summary ==="
echo "${GREEN}Passed: $PASS${NC}"
echo "${RED}Failed: $FAIL${NC}"
echo ""

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
