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
ORG_ID=your-org-id make test-property
```

## Properties Tested (54 total)
- **10 Template properties** - Parsing, security, consistency
- **8 UUID properties** - Uniqueness, format, ordering
- **10 Retry properties** - Backoff, limits, headers
- **8 Circuit Breaker properties** - Thresholds, states, recovery
- **10 Filter properties** - Operators, serialization, types
- **8 Data/Report properties** - Integrity, timestamps, format

## Iterations
~3,600 random inputs tested per run
