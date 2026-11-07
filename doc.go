// Package tfplansummary parses Terraform plan JSON output (terraform show -json),
// ranks resource changes by risk level, and renders clear, safe summaries in
// Markdown or JSON without leaking secret values.
package tfplansummary
