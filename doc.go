// Package tfplansummary is a small, dependency-free Go library: Summarize terraform show -json plans by risk: replace and delete detection, protected resource rules, attribute names without values, Markdown or JSON
//
// The public API is every exported identifier of this root package. Functions are pure: they do
// no I/O, keep no global mutable state and report invalid input through returned errors.
package tfplansummary
