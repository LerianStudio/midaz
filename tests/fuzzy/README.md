Fuzz tests target inputs with random and edge-case values, validating robust parsing and error handling.

Run fuzzers locally with:
  go test -v ./tests/fuzzy -fuzz=Fuzz -run=^$

