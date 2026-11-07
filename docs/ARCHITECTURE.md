# Architecture

`tf-plan-summary` processes Terraform plan JSON streams and calculates risk summaries.

## Core Components

1. **Parser (`Parse`)**:
   - Reads JSON using an `io.LimitReader` bounded by `Options.MaxBytes` (default 50 MiB).
   - Validates major version `1.x` of the Terraform JSON output specification.
   - Unmarshals resource changes and maps action combinations:
     - `["create"]` -> Create (Low)
     - `["update"]` -> Update (Medium)
     - `["delete"]` -> Delete (High)
     - `["delete", "create"]` / `["create", "delete"]` -> Replace (High)
     - `["read"]` and `["no-op"]` are skipped.

2. **Attribute Diffing Engine**:
   - Inspects `before` and `after` maps with `reflect.DeepEqual` on decoded values.
   - Incorporates `after_unknown` attributes formatted as `<name> (known after apply)`.
   - Checks `before_sensitive` and `after_sensitive` trees (recursively scanning map and slice leaves) to flag sensitive attributes with ` (sensitive)`.
   - Retains only attribute names; attribute values are discarded immediately to guarantee no values leak into output or logs.

3. **Risk Policy Evaluator**:
   - Assigns a base level per action.
   - Evaluates glob rules against `rc.Type` using `path.Match`.
   - Maintains the invariant that custom rules can elevate risk, but never lower the base action level.

4. **Formatters**:
   - `RenderMarkdown`: produces GitHub-compatible Markdown summary lines and tables, escaping pipe `|` characters in resource addresses and names, truncating attribute diffs over 20 items to `+N more`.
   - JSON serialization: lowercase snake_case JSON struct tags.
