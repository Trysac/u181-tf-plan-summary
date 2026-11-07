# tf-plan-summary

[![CI](https://github.com/Trysac/u181-tf-plan-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/Trysac/u181-tf-plan-summary/actions/workflows/ci.yml)

Summarize `terraform show -json` plans by risk: replace and delete detection, protected resource rules, attribute names without values, Markdown or JSON output.

## Overview

Terraform plans for real infrastructure run to thousands of lines. Reviewers routinely miss the one database that will be replaced or the IAM change hidden among tag updates. Raw plan output also prints values, including secrets, into CI logs and pull request comments.

`tf-plan-summary` reads the standard JSON plan output, ranks changes by configurable risk policies, detects replacements and deletes, and renders a concise summary that lists attribute names but never secret or raw values.

## Installation

```bash
go install github.com/Trysac/u181-tf-plan-summary/cmd/tf-plan-summary@latest
```

## Usage

```go
planJSON := `{"format_version":"1.0","resource_changes":[{"address":"aws_s3_bucket.b","type":"aws_s3_bucket","change":{"actions":["delete"]}}]}`
summary, err := tfplansummary.Parse(strings.NewReader(planJSON), tfplansummary.Options{})
if err != nil {
	log.Fatal(err)
}
fmt.Println(tfplansummary.RenderMarkdown(summary))
```

## API Summary

- `Parse(io.Reader, Options) (*Summary, error)`: Parses JSON input. Limits read bytes to `Options.MaxBytes` (default 52,428,800). Returns `ErrTooLarge`, `ErrInvalidPlan`, or `ErrUnsupportedFormat`.
- `RenderMarkdown(*Summary) string`: Outputs risk-ranked Markdown summary with escaped pipe characters.
- `Run([]string, io.Writer, io.Writer) int`: CLI entry point handling arguments like `--fail-on` and `--format`.

## Limits and Options

- **Input**: Terraform `format_version` major 1.
- **Risk Levels**: None, Low, Medium, High, Critical.
- **Action Mapping**: `["create"]` (Low), `["update"]` (Medium), `["delete"]` (High), `["delete","create"]`/`["create","delete"]` (Replace/High). Reads/No-ops are ignored.

## Why not tf-summarize or OPA?

Unlike alternatives, this utility requires zero installation outside Go, is safe for public logs because it never extracts values, and manages risk levels as plain Go data.

See [docs/SECURITY.md](docs/SECURITY.md).