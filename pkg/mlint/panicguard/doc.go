// Package panicguard provides go/analysis analyzers for enforcing panic hardening
// patterns in the Midaz codebase.
//
// This package implements four analyzers:
//
//   - NoRawGoroutine: Detects raw 'go' statements and requires mruntime.SafeGo()
//   - NoRecoverOutsideBoundary: Restricts recover() to boundary packages only
//   - NoPanicInProduction: Warns against panic() calls in production code
//   - NoBareRecover: Ensures recover() calls capture and log the panic value
//
// These analyzers can be run standalone or integrated with golangci-lint.
//
// # Exclusion Patterns
//
// Each analyzer supports path-based exclusions for:
//   - pkg/mruntime/ - The runtime safety package itself
//   - *_test.go - Test files
//   - *.pb.go, *_mock.go, mock_*.go, mocks/ - Generated and mock files
//   - Boundary packages (for recover rules): adapters/http/, adapters/grpc/, etc.
//
// # Usage with golangci-lint
//
// Configure in .golangci.yml:
//
//	linters:
//	  enable:
//	    - panicguard
//	  settings:
//	    custom:
//	      panicguard:
//	        type: module
//	        description: Panic hardening enforcement
package panicguard
