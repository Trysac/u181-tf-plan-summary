# Testing Strategy

Tests are located in `tfplansummary_test.go` and use package `tfplansummary_test`. Tests operate strictly on JSON fixtures in `testdata`.

## Covered Scenarios

- **Parsing**: Validates `Parse` logic for mixed resource actions (create, update, delete, replace, read/no-op) and version enforcement (rejects `format_version` 2).
- **Risk Evaluation**: Verifies that rules properly elevate risk but cannot lower base action risk; validates default rule critical/high risk thresholds.
- **Attribute Diffing**: Ensures attribute keys are correctly identified, sensitive masks applied, and that values are stripped. Checks that unknown attributes get `(known after apply)` suffix.
- **Markdown/JSON**: Validates rendering of tables, escaping of `|`, and truncation of attribute lists exceeding 20 items.
- **Error Handling**: Confirms accurate `errors.Is` checks for `ErrTooLarge`, `ErrInvalidPlan`, and `ErrUnsupportedFormat`.
- **CLI**: Integration tests check `Run` exit codes (0 for success, 1 for risk threshold breach, 2 for usage/io errors) and CLI flag parsing.