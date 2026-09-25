// Package main is the composition root of gruntled. It is where adapters
// (CLI, filesystem, HCL parsing) get wired together into the running
// binary. Wiring happens in Phase 3; this Phase 1 stub only exists so that
// `go build ./...` exercises the full module tree, including the domain
// packages.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "gruntled: no commands implemented yet")
	os.Exit(1)
}
