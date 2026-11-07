package main

import (
	"os"

	tfplansummary "github.com/Trysac/u181-tf-plan-summary"
)

func main() {
	code := tfplansummary.Run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
