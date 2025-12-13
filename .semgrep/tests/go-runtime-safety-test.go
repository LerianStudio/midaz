// This file contains code patterns that should be caught by Semgrep rules.
// Run: semgrep --config ../ .
// Expected: Multiple findings for each rule

package tests

import "fmt"

// =============================================================================
// Tests for go-no-raw-goroutine
// =============================================================================

// ruleid: go-no-raw-goroutine
func badRawGoroutine() {
	go func() {
		fmt.Println("This should be caught")
	}()
}

// ruleid: go-no-raw-goroutine
func badRawGoroutineWithFunction() {
	go doSomething()
}

func doSomething() {}

// =============================================================================
// Tests for go-no-recover-outside-boundary
// =============================================================================

// Note: This file is NOT in an excluded path, so recover() should be caught

// ruleid: go-no-recover-outside-boundary
func badRecoverOutsideBoundary() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Caught:", r)
		}
	}()
	panic("test")
}

// =============================================================================
// Tests for go-no-panic-in-production
// =============================================================================

// ruleid: go-no-panic-in-production
func badPanicInProduction() {
	panic("this should not be in production code")
}

// ruleid: go-no-panic-in-production
func badPanicWithFormat() {
	x := 42
	panic(fmt.Sprintf("unexpected value: %d", x))
}

// =============================================================================
// Tests for go-no-bare-recover
// =============================================================================

// ruleid: go-no-bare-recover
func badBareRecover() {
	defer recover() // Silently swallows panic
}

// ruleid: go-no-bare-recover
func badDiscardedRecover() {
	defer func() {
		_ = recover() // Discards panic value
	}()
}

// =============================================================================
// OK patterns (should NOT be caught)
// =============================================================================

// ok: go-no-recover-outside-boundary (would be caught, but demonstrating pattern)
// Note: This WILL be caught because this file is not in an excluded directory.
// In real code, this pattern would be OK in pkg/mruntime/ or adapters.
func okRecoverWithLogging() {
	defer func() {
		if r := recover(); r != nil {
			// This is the correct pattern - capture and use r
			fmt.Printf("panic recovered: %v\n", r)
		}
	}()
}
