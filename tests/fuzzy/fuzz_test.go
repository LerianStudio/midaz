package fuzzy

import "testing"

// FuzzCreateOrganizationName will fuzz organization names to validate input handling.
// Run with: go test -v ./tests/fuzzy -fuzz=Fuzz -run=^$
func FuzzCreateOrganizationName(f *testing.F) {
    f.Skip("implementation pending: connect to API and assert behaviors")
    // Seed inputs for corpus
    f.Add("Acme, Inc.")
    f.Add("")
    f.Add("a")

    f.Fuzz(func(t *testing.T, name string) {
        _ = name
        // TODO: call onboarding create organization with fuzzed name
        // and validate status code and error handling
    })
}

