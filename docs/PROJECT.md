# Project Record

`tf-plan-summary` is a lightweight, zero-dependency Go library and CLI for extracting risk indicators and summaries from Terraform JSON plans.

## Objective

To provide a mechanism for CI pipelines to safely evaluate infrastructure changes without leaking sensitive values, by strictly outputting attribute names and risk levels.

## Scope

- Parsing Terraform JSON plan files (`format_version` 1).
- Detection of resource replacements and deletes.
- Assigning risk levels via configurable rulesets.
- Generating human-readable Markdown or machine-readable JSON summaries.

## Out of Scope

- Executing Terraform binaries or managing plan generation.
- Detecting drift (resource_drift).
- Cost estimation.
- Complex policy-as-code languages (e.g., Rego).
- Detecting modifications to Terraform output variables.

## Factory run u181 (measured from API usage)

| Phase | Model | Batch | Input | Output | Cache write | Cache read | Cost (USD) |
|---|---|---|---|---|---|---|---|
| generation | gemini-3.8-flash | yes | 4560 | 51840 | 0 | 0 | 0.0989 |
| audit | gemini-3.1-flash-lite | no | 24321 | 307 | 0 | 10191 | 0.0068 |
| docs | gemini-3.1-flash-lite | no | 4489 | 1458 | 0 | 0 | 0.0033 |

Total: $0.1090. Prices: Anthropic list prices.
