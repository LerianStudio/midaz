# Property-Based Tests

## Purpose
Validate mathematical invariants and behavioral properties across all possible inputs.

## What We Test
- **Templates** - Determinism, idempotency, safety
- **UUIDs** - Uniqueness, monotonicity, format
- **Retry/Backoff** - Exponential growth, limits, consistency
- **Circuit Breaker** - Threshold behavior, state transitions
- **Filters** - Operator validity, JSON serialization
- **Data Integrity** - Timestamps, metadata, consistency

## Key Guarantees
✅ **Deterministic** - Same input → Same output (always)  
✅ **Idempotent** - f(f(x)) = f(x)  
✅ **No duplicates** - UUIDs always unique  
✅ **Bounded retries** - Never infinite loops  
✅ **Valid states** - Circuit breaker transitions correct  

## Running
```bash
ORG_ID=your-org-id go test -tags property ./tests/reporter/property/...
```

## Properties Tested (70 total)
- **17 Deadline properties** - ComputeStatus trichotomy, delivered-dominance, overdue/pending, NewDeadline validation
- **10 Filter properties** - Operators, serialization, types
- **10 Retry properties** - Backoff, limits, headers
- **9 Template properties** - Parsing, security, consistency
- **8 UUID properties** - Uniqueness, format, ordering
- **8 Circuit Breaker properties** - Thresholds, states, recovery
- **5 Data Integrity properties** - Timestamps, metadata, consistency
- **3 Report Status properties** - Status transitions, invariants

## Iterations
~5,900 random inputs tested per run
