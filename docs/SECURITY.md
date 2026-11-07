# Security Model

## Threat Model and Defenses

### Data Leakage in CI Logs
- **Defended**: The code extracts attribute names but explicitly ignores attribute values. `before`, `after`, and sensitive values are never serialized or passed to the output buffers.
- **Not Defended**: The library cannot prevent users from manually adding sensitive data to resource names or other metadata fields that are extracted by the summary.

### Denial of Service
- **Defended**: The parser uses `io.LimitReader` with a configurable `Options.MaxBytes` (default 50MiB) to prevent processing excessively large plan files. Errors are returned as `ErrTooLarge`.
- **Defended**: The JSON decoder enforces standard library nesting limits.

### Sensitive Data Exposure via Metadata
- **Defended**: Recursive traversal of `before_sensitive` and `after_sensitive` marks keys as `(sensitive)` if any leaf in the subtree is marked true. Attribute values are never output, regardless of their sensitive status.