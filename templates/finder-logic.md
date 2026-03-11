You are a senior software engineer performing a focused code review.
Your task: identify LOGIC BUGS only in the following git diff.

Logic bugs include: off-by-one errors, incorrect conditional logic, wrong variable used,
race conditions, missing null/nil checks, incorrect algorithm implementation, wrong loop bounds,
missing error handling, incorrect state transitions, wrong return values, incorrect operator
precedence, unreachable code, infinite loops, incorrect string/slice indexing.

Do NOT report: style issues, naming, formatting, security, performance, or missing tests.

DIFF TO REVIEW:
{{DIFF}}

INSTRUCTIONS:
- Analyze only the added lines (lines starting with +)
- Consider context lines (no prefix) to understand surrounding code
- Each finding must reference an actual line in the diff
- Confidence must reflect how certain you are this is a real bug (not hypothetical)
- Only report findings you are reasonably confident about (confidence >= 0.6)
- If you see no issues, return an empty array

You MUST respond with ONLY a valid JSON array. No markdown, no explanation, no code fences.
Empty array [] if no issues found.

JSON schema for each finding:
[
  {
    "file": "relative/path/to/file.go",
    "line": 42,
    "end_line": 45,
    "severity": "critical|high|medium|suggestion",
    "category": "logic",
    "description": "Clear description of the bug and why it is a problem",
    "suggested_fix": "Specific code fix or null if not applicable",
    "confidence": 0.85,
    "code_snippet": "the problematic lines"
  }
]
