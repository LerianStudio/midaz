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
```bash
# All fuzzing tests (1 minute each)
ORG_ID=your-org-id make test-fuzzy

# Specific test
go test -fuzz=FuzzTemplateInvalidTags -fuzztime=30s ./tests/fuzzy
```

## Test Files
- `fuzz_template_invalid_tags_test.go` - Non-existent fields/tables
- `fuzz_template_syntax_test.go` - Syntax errors, XSS, sizes
- `fuzz_filters_test.go` - Malformed filters
- `fuzz_report_payload_test.go` - Invalid payloads, IDs
- `fuzz_null_payload_test.go` - Null/empty validation
- `fuzz_predefined_invalid_templates_test.go` - Specific error scenarios
