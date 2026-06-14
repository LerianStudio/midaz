# Fuzzing Tests

## Purpose
Test system robustness against malformed, invalid, and malicious inputs.

## What We Test
- **Invalid templates** - Non-existent fields, tables, databases
- **Malformed syntax** - Unclosed tags, broken expressions
- **Invalid filters** - Malformed operators, SQL injection attempts
- **Invalid payloads** - Null, empty, wrong types
- **Security** - XSS attempts, script injection, path traversal
- **Edge cases** - Extreme sizes, deep nesting, special characters

## Key Guarantees
✅ **No server crashes** - Never returns 5xx errors  
✅ **Graceful errors** - Invalid inputs return 4xx with clear messages  
✅ **XSS protection** - Script tags always blocked  
✅ **No panics** - Any input handled safely  

## Running

This suite is gated by the `fuzz` build tag and spins up infrastructure via
testcontainers by default (set `USE_EXISTING_INFRA=true` to reuse a running
docker-compose stack instead).

```bash
# Run this suite's Fuzz* targets (build tag + full package path required)
go test -tags fuzz ./tests/reporter/fuzzy/...

# Specific target, e.g. 30s
go test -tags fuzz -fuzz=FuzzTemplate_InvalidTags -fuzztime=30s ./tests/reporter/fuzzy
```

> Note: `make test-fuzz` now discovers and runs this suite (it scans
> `./components ./pkg ./tests`, with the larger FUZZTIME_FUZZY budget); direct
> `go test` is still fine for a single target.

## Test Files
- `fuzz-template-invalid-tags_test.go` - Non-existent fields/tables
- `fuzz-template-syntax_test.go` - Syntax errors, XSS, sizes
- `fuzz-blocks-config_test.go` - Malformed blocks-config requests/bodies, header injection
- `fuzz-filters_test.go` - Malformed filters config
- `fuzz-report-payload_test.go` - Invalid report payloads, organization IDs
- `fuzz-deadline-payload_test.go` - Malformed deadline create/update/deliver payloads
- `fuzz-null-payload_test.go` - Null/empty validation
- `fuzz-predefined-invalid-templates_test.go` - Specific error scenarios
- `fuzz_test.go` / `main_test.go` - Shared fuzz harness and testcontainers setup
