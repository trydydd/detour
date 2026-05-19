#!/bin/bash
# Verify that args passed after -- on the docker run command line reach
# the claude subprocess correctly.
#
# Strategy: build a derived image where /usr/local/bin/claude is replaced
# by a shim that writes "$@" to a volume-mounted host file, then inspect
# that file after the container exits.
#
# Exit codes:
#   0 = all checks passed
#   1 = one or more checks failed

set -eu

IMAGE="${IMAGE:-detour:test}"
TEST_IMAGE="${TEST_IMAGE:-detour:test-args}"

PASS=0
FAIL=0

if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  NC='\033[0m'
else
  RED=''
  GREEN=''
  NC=''
fi

pass() { echo "${GREEN}PASS${NC}: $1"; PASS=$((PASS + 1)); }
fail() { echo "${RED}FAIL${NC}: $1"; FAIL=$((FAIL + 1)); }

ARGV_FILE1="/tmp/detour-argv-test1-$$"
ARGV_FILE2="/tmp/detour-argv-test2-$$"
SHIM_DOCKERFILE="/tmp/Dockerfile.shim.$$"

cleanup() {
  docker rmi -f "$TEST_IMAGE" 2>/dev/null || true
  rm -f "$ARGV_FILE1" "$ARGV_FILE2" "$SHIM_DOCKERFILE"
}
trap cleanup EXIT

echo "=== Claude Args Pass-Through Test ==="
echo "Base image: $IMAGE"
echo "Test image: $TEST_IMAGE"
echo ""

# Build derived image with a claude shim that records its argv
echo "--- Building derived image with claude shim ---"
cat > "$SHIM_DOCKERFILE" <<'EOF'
ARG BASE_IMAGE=detour:test
FROM ${BASE_IMAGE}
RUN printf '#!/bin/sh\necho "$@" > /tmp/claude-argv\n' > /usr/local/bin/claude \
 && chmod +x /usr/local/bin/claude
EOF

if docker build --build-arg "BASE_IMAGE=$IMAGE" -t "$TEST_IMAGE" \
    -f "$SHIM_DOCKERFILE" . >/dev/null 2>&1; then
  pass "Derived image built with claude shim"
else
  fail "Failed to build derived image"
  exit 1
fi

# Test 1: Args pass-through -- --dangerously-skip-permissions --version
# The entrypoint strips the leading --, then passes the remaining args to
# detour, which passes them to the claude shim.
echo ""
echo "--- Test 1: Args pass-through (--dangerously-skip-permissions --version) ---"
touch "$ARGV_FILE1" && chmod 666 "$ARGV_FILE1"
docker run --rm \
  -v "$ARGV_FILE1:/tmp/claude-argv" \
  -e DETOUR_MODEL_NAME=dummy \
  -e DETOUR_MODEL_API=http://127.0.0.1:9 \
  "$TEST_IMAGE" -- --dangerously-skip-permissions --version >/dev/null 2>&1 || true

ARGV1=$(tr -d '\n' < "$ARGV_FILE1" 2>/dev/null || echo "")
if [ "$ARGV1" = "--dangerously-skip-permissions --version" ]; then
  pass "Args passed correctly: '$ARGV1'"
else
  fail "Args mismatch. Expected '--dangerously-skip-permissions --version', got: '$ARGV1'"
fi

# Test 2: Empty args — no args after image name → claude receives no args
echo ""
echo "--- Test 2: Empty args (claude receives no args) ---"
touch "$ARGV_FILE2" && chmod 666 "$ARGV_FILE2"
docker run --rm \
  -v "$ARGV_FILE2:/tmp/claude-argv" \
  -e DETOUR_MODEL_NAME=dummy \
  -e DETOUR_MODEL_API=http://127.0.0.1:9 \
  "$TEST_IMAGE" >/dev/null 2>&1 || true

ARGV2=$(tr -d '\n' < "$ARGV_FILE2" 2>/dev/null || echo "")
if [ -z "$ARGV2" ]; then
  pass "Empty args: claude received no args"
else
  fail "Expected empty args, got: '$ARGV2'"
fi

# Summary
echo ""
echo "=== Test Summary ==="
echo "${GREEN}Passed: $PASS${NC}"
echo "${RED}Failed: $FAIL${NC}"
echo ""
if [ "$FAIL" -gt 0 ]; then exit 1; fi
exit 0
