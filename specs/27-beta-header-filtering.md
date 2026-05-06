# 27. Anthropic-Beta Header Filtering Details

## Topic Statement

Filter the `Anthropic-Beta` header by removing thinking-related tokens while preserving other beta flags when forwarding to local inference servers.

**Related specs:**
- Spec 14 (Header Filtering and Forwarding) - covers which headers are forwarded to each backend
- Spec 05 (Thinking Block Stripping) - covers thinking removal from request/response bodies

## Scope

**In-scope:**
- Parsing comma-separated beta tokens
- Filtering tokens containing "thinking" substring
- Reconstructing the filtered header value
- Edge case handling for malformed or empty results

**Boundaries:**
- Input: Original `Anthropic-Beta` header value
- Output: Filtered header value (may be empty or malformed)
- Only applies to local forwarding (DoLocal path)

## Data Contracts

### Input Format

```
Anthropic-Beta: <token1>, <token2>, <token3>, ...
```

Tokens are comma-separated. Whitespace around tokens may vary.

### Output Format

```
Anthropic-Beta: <filtered_tokens>
```

Filtered tokens joined with comma and space separator.

## Behaviors

### Filtering Sequence

1. Split header value by comma character (`,`)
2. For each token:
   - Trim leading and trailing whitespace
   - Check if token contains "thinking" substring (case-sensitive)
   - If contains "thinking": exclude from output
   - If does not contain "thinking": include in output
3. Join remaining tokens with comma separator
4. Return joined string (may be empty or have leading/trailing commas in edge cases)

### Filtering Algorithm

```
filterThinkingBeta(v string) string:
    parts = split v by ","
    kept = empty list
    for each p in parts:
        trimmed = trim_whitespace(p)
        if "thinking" not in trimmed:
            append trimmed to kept
    return join kept with ","
```

## State Transitions

| Input | Tokens Filtered | Output |
|-------|-----------------|--------|
| `interleaved-thinking-2025-05-14` | 1 (all) | `` (empty) |
| `interleaved-thinking-2025-05-14,prompt-caching-2024-07-31` | 1 of 2 | ` prompt-caching-2024-07-31` |
| `prompt-caching-2024-07-31` | 0 | `prompt-caching-2024-07-31` |
| `` (empty) | 0 | `` (empty) |

## Notable Behaviors

1. **Leading space preserved**: When the first token is filtered, the output may have a leading space (e.g., `thinking-1,cache-2` becomes `,cache-2` or ` cache-2` depending on trimming)

2. **No re-joining normalization**: The function joins with comma only, not comma-space. Original spacing is lost.

3. **Empty result possible**: If all tokens contain "thinking", the result is an empty string

4. **Case-sensitive matching**: Only lowercase "thinking" is matched. "Thinking" or "THINKING" would not be filtered.

5. **Substring matching**: Any token containing "thinking" anywhere is filtered (e.g., "thinking-2025", "interleaved-thinking", "my-thinking-token" all filtered)

## Error Handling

| Condition | Behavior |
|-----------|----------|
| Empty input | Return empty string |
| No commas (single token) | Return token unchanged if no "thinking", empty if contains "thinking" |
| Trailing comma | Trailing empty token trimmed, no trailing comma in output |
| Leading comma | Leading empty token trimmed, no leading comma in output |
| Multiple consecutive commas | Empty tokens between commas filtered out |

## Rationale

Beta header filtering serves several purposes:

1. **Compatibility**: Local inference servers may not support thinking-related beta features; sending unsupported flags could cause errors

2. **Prevention of invalid signatures**: Thinking beta flags may cause local servers to generate thinking blocks with invalid signatures, breaking subsequent passthrough requests

3. **Clean separation**: Ensures local servers only receive beta features they are expected to support

## Examples

### Example 1: Single Thinking Token

**Input:**
```
Anthropic-Beta: interleaved-thinking-2025-05-14
```

**Output:**
```
Anthropic-Beta: 
```

Result is empty string (all tokens filtered)

### Example 2: Mixed Tokens

**Input:**
```
Anthropic-Beta: interleaved-thinking-2025-05-14, prompt-caching-2024-07-31
```

**Output:**
```
Anthropic-Beta:  prompt-caching-2024-07-31
```

Note: Leading space preserved from original token separation

### Example 3: No Thinking Tokens

**Input:**
```
Anthropic-Beta: prompt-caching-2024-07-31, tool-use-2024-01-01
```

**Output:**
```
Anthropic-Beta: prompt-caching-2024-07-31,tool-use-2024-01-01
```

All tokens preserved, spacing normalized (joined with comma, no spaces)

### Example 4: Empty Header

**Input:**
```
Anthropic-Beta: 
```

**Output:**
```
Anthropic-Beta: 
```

Empty input returns empty output

## Testing Scenarios

### Single Thinking Token
- Input: `interleaved-thinking-2025-05-14`
- Expected: Empty string

### Mixed Tokens
- Input: `thinking-1, cache-2`
- Expected: ` cache-2` (leading space from split)

### All Thinking Tokens
- Input: `thinking-1, thinking-2`
- Expected: Empty string (all filtered)

### No Thinking Tokens
- Input: `cache-1, tools-2`
- Expected: `cache-1,tools-2` (no spaces in output)

### Edge Case: Trailing Comma
- Input: `cache-1,`
- Expected: `cache-1` (trailing empty token removed)

### Edge Case: Leading Comma
- Input: `,cache-1`
- Expected: `cache-1` (leading empty token removed)

## Implementation Notes

The filtering function `filterThinkingBeta()` is called only for local routes (DoLocal path). For passthrough routes to Anthropic, the `Anthropic-Beta` header is forwarded unchanged without filtering.
