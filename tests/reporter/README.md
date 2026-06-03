# Reporter Test Suite

## Overview
Comprehensive testing strategy covering unit, integration, property-based, fuzzing, and chaos scenarios.

---

## Test Categories

### 📁 **chaos/** - Infrastructure Resilience
**Purpose:** Validate recovery from catastrophic failures

**Tests:**
- RabbitMQ crashes
- Worker crashes  
- Database failures
- Network partitions

**Guarantees:** Zero message loss, auto-recovery

```bash
ORG_ID=your-org-id make test-chaos
```

---

### 📁 **fuzzy/** - Input Validation & Security
**Purpose:** Find bugs with random malformed inputs

**Tests:**
- Invalid templates
- Malformed payloads
- XSS/SQL injection attempts
- Extreme inputs (size, nesting, special chars)

**Guarantees:** No crashes (5xx), graceful error handling

```bash
ORG_ID=your-org-id make test-fuzzy
```

---

### 📁 **property/** - Mathematical Invariants
**Purpose:** Prove behavioral properties hold for all inputs

**Tests:**
- Template determinism & idempotency
- UUID uniqueness & ordering
- Retry exponential backoff
- Circuit breaker state transitions
- Filter operator consistency

**Guarantees:** Predictable behavior, no edge case bugs

```bash
ORG_ID=your-org-id make test-property
```

---

### 📁 **integration/** - End-to-End Workflows
**Purpose:** Validate complete user journeys

**Tests:**
- Template → Report generation
- CRUD operations
- Multi-datasource queries
- Status progression
- File storage

**Guarantees:** Components work together correctly

```bash
ORG_ID=your-org-id make test-integration
```

---

## Quick Start

### Run All Tests
```bash
ORG_ID=your-org-id make test-all
```

### Run Specific Category
```bash
ORG_ID=your-org-id make test-chaos
ORG_ID=your-org-id make test-fuzzy
ORG_ID=your-org-id make test-property
ORG_ID=your-org-id make test-integration
```

### Configuration
Set `ORG_ID` or `X_ORGANIZATION_ID` environment variable:
```bash
export ORG_ID=06c4f684-19b0-449a-81f4-f9a4e503db83
make test-chaos
```

---

## Test Pyramid

```
         /\
        /  \  Unit Tests (fast, isolated)
       /----\
      / Inte \  Integration Tests (moderate, real components)
     /--------\
    / Property \  Property-Based (mathematical guarantees)
   /------------\
  /    Fuzzy     \  Fuzzing (security, robustness)
 /----------------\
/      Chaos       \  Chaos Engineering (infrastructure resilience)
--------------------
```

---

## Coverage Summary

| Category | Tests | Iterations | Time | Purpose |
|----------|-------|------------|------|---------|
| **Chaos** | 11 | - | ~3min | Infrastructure failures |
| **Fuzzy** | 162 | 100k+ | ~1s | Input validation |
| **Property** | 53 | ~3.6k | ~2s | Invariants |
| **Integration** | varies | - | varies | E2E workflows |

**Total:** 200+ tests ensuring system reliability

---

## When to Use Each Test Type

### Use **Chaos** when:
- Testing disaster recovery
- Validating message persistence
- Ensuring auto-healing works

### Use **Fuzzy** when:
- Finding security vulnerabilities
- Testing input validation
- Discovering edge cases

### Use **Property** when:
- Proving mathematical correctness
- Ensuring consistency
- Preventing regression

### Use **Integration** when:
- Validating user workflows
- Testing component interactions
- Verifying business logic

---

**All tests work together to ensure Reporter is secure, reliable, and resilient.**

