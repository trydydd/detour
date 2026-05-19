#!/bin/bash
# Usage: ./loop.sh [mode] [iterations]
# Modes: build, plan, specs
# Examples:
#   ./loop.sh build 10     # Build mode, 10 iterations
#   ./loop.sh plan 5       # Plan mode, 5 iterations
#   ./loop.sh specs 20     # Specs mode, 20 iterations

# Validate required arguments
if [ -z "$1" ]; then
    echo "Error: Mode is required. Usage: ./loop.sh [mode] [iterations]"
    echo "Modes: build, plan, specs"
    exit 1
fi

if [ -z "$2" ]; then
    echo "Error: Iterations count is required. Usage: ./loop.sh [mode] [iterations]"
    exit 1
fi

# Parse arguments
case "$1" in
    build|plan|specs)
        MODE="$1"
        PROMPT_FILE=".ralph/PROMPT_${MODE}.md"
        MAX_ITERATIONS="$2"
        ;;
    *)
        echo "Error: Invalid mode '$1'. Valid modes are: build, plan, specs"
        exit 1
        ;;
esac

ITERATION=0
CURRENT_BRANCH=$(git branch --show-current)

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Mode:   $MODE"
echo "Prompt: $PROMPT_FILE"
echo "Branch: $CURRENT_BRANCH"
[ $MAX_ITERATIONS -gt 0 ] && echo "Max:    $MAX_ITERATIONS iterations"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Verify prompt file exists
if [ ! -f "$PROMPT_FILE" ]; then
    echo "Error: $PROMPT_FILE not found"
    exit 1
fi

while true; do
    if [ $MAX_ITERATIONS -gt 0 ] && [ $ITERATION -ge $MAX_ITERATIONS ]; then
        echo "Reached max iterations: $MAX_ITERATIONS"
        break
    fi

    # Run Ralph iteration with selected prompt
    # -p: Headless mode (non-interactive, reads from stdin)
    # --dangerously-skip-permissions: Auto-approve all tool calls (YOLO mode)
    # --output-format=stream-json: Structured output for logging/monitoring
    # --model red: Local inference server alias
    # --verbose: Detailed execution logging
    # cat "$PROMPT_FILE" | claude -p \
    #     --dangerously-skip-permissions \
    #     --output-format=stream-json \
    #     --model red \
    #     --verbose
    PROMPT_TEXT=$(cat $PROMPT_FILE)

    IS_SANDBOX=1 cat $PROMPT_FILE | ~/go/bin/detour \
        --model-name red \
        --model-api http://192.168.0.214:8000 -- \
        --dangerously-skip-permissions \
        --model red \
        --output-format=stream-json \
        --verbose \
        -p "$PROMPT_TEXT"


    # Push changes after each iteration
    git push origin "$CURRENT_BRANCH" || {
        echo "Failed to push. Creating remote branch..."
        git push -u origin "$CURRENT_BRANCH"
    }

    ITERATION=$((ITERATION + 1))
    echo -e "\n\n======================== LOOP $ITERATION ========================\n"
done
