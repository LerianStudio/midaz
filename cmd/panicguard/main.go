// Package main provides a standalone CLI for panicguard analyzers.
//
// This allows running the panic hardening linters outside of golangci-lint.
//
// Usage:
//
//	panicguard ./...
//	panicguard -fix ./...
//	panicguard ./components/transaction/...
package main

import (
	"github.com/LerianStudio/midaz/v3/pkg/mlint/panicguard"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(panicguard.Analyzers()...)
}
