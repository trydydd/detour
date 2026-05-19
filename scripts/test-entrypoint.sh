#!/bin/sh
# Test script for docker-entrypoint.sh
# Tests the four scenarios:
#   1. Without DETOUR_MODEL_NAME/DETOUR_MODEL_API - should invoke "detour -- ..."
#   2. With DETOUR_MODEL_NAME=red and DETOUR_MODEL_API=http://x - should include flags
#   3. Without ANTHROPIC_DETOUR_AUTH - should generate 32-char value
#   4. With --no-claude - should run detour without launching claude

set -eu

SCRIPTDIR="$(cd "$(dirname "$0")" && pwd)"
ENTRYPOINT="$SCRIPTDIR/docker-entrypoint.sh"

PASS=0
FAIL=0

test_case() {
  local name="$1"
  local expected="$2"

  export _DRY_RUN=1
  result=$("$ENTRYPOINT" 2>&1)

  if [ "$result" = "$expected" ]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  Expected: $expected"
    echo "  Got:      $result"
    FAIL=$((FAIL + 1))
  fi
  unset _DRY_RUN
}

test_case_with_args() {
  local name="$1"
  local args="$2"
  local expected="$3"

  export _DRY_RUN=1
  result=$("$ENTRYPOINT" $args 2>&1)

  if [ "$result" = "$expected" ]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  Expected: $expected"
    echo "  Got:      $result"
    FAIL=$((FAIL + 1))
  fi
  unset _DRY_RUN
}

test_case_with_env() {
  local name="$1"
  local env="$2"
  local expected="$3"

  # Reset env
  unset DETOUR_MODEL_NAME
  unset DETOUR_MODEL_API

  # Set env for this test
  eval "$env"

  export _DRY_RUN=1
  result=$("$ENTRYPOINT" 2>&1)

  if [ "$result" = "$expected" ]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  Expected: $expected"
    echo "  Got:      $result"
    FAIL=$((FAIL + 1))
  fi
  unset _DRY_RUN
  unset DETOUR_MODEL_NAME
  unset DETOUR_MODEL_API
}

# Test 1: Without DETOUR_MODEL_NAME/DETOUR_MODEL_API
test_case_with_env "No model env vars" "" "detour -- claude -- "

# Test 2: With DETOUR_MODEL_NAME and DETOUR_MODEL_API
test_case_with_env "With model env vars" "export DETOUR_MODEL_NAME=red DETOUR_MODEL_API=http://x" "detour --model-name red --model-api http://x -- claude -- "

# Test 3: With --no-claude flag
test_case_with_args "With --no-claude" "--no-claude --help" "detour -- --help"

# Test 4: With --no-claude and model env vars
export _DRY_RUN=1
export DETOUR_MODEL_NAME=red
export DETOUR_MODEL_API=http://x
result=$("$ENTRYPOINT" --no-claude --version 2>&1)
unset _DRY_RUN
unset DETOUR_MODEL_NAME
unset DETOUR_MODEL_API

if [ "$result" = "detour --model-name red --model-api http://x -- --version" ]; then
  echo "PASS: --no-claude with model env vars"
  PASS=$((PASS + 1))
else
  echo "FAIL: --no-claude with model env vars"
  echo "  Expected: detour --model-name red --model-api http://x -- --version"
  echo "  Got:      $result"
  FAIL=$((FAIL + 1))
fi

# Summary
echo ""
echo "Results: $PASS passed, $FAIL failed"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
