# Risk-based Terraform plan summarization without value retention

Status: accepted (generated, pending human review)

## Context

Terraform plans contain extensive details that overwhelm reviewers and risk leaking credentials or secrets into CI logs and PR comments. We needed a safe, zero-dependency analyzer that identifies risk levels, deletes, and replacements without retaining values.

## Decision

Implement tfplansummary with strict action mapping, base action risk floors, recursive sensitive mark evaluation, and attribute name extraction that completely discards before and after values.

## Consequences

Outputs in Markdown and JSON remain clean and safe for public logs, while high-risk database deletions and IAM modifications are prominently highlighted.
