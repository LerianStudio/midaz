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
ORG_ID=your-org-id go test -tags=chaos ./tests/reporter/chaos/...
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
ORG_ID=your-org-id go test -tags=fuzz ./tests/reporter/fuzzy/...
```

> Note: the directory is `fuzzy/` but the build tag is `fuzz` (not `fuzzy`). Using `-tags=fuzzy` silently compiles zero files.

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
ORG_ID=your-org-id go test -tags=property ./tests/reporter/property/...
```

---

### 📁 **integration/** - Component Integration
**Purpose:** Validate components against real dependencies (testcontainers)

**Tests:**
- Template → Report generation
- CRUD operations
- Multi-datasource queries
- Status progression
- File storage

**Guarantees:** Components work together correctly

```bash
ORG_ID=your-org-id go test -tags=integration ./tests/reporter/integration/...
```

---

### 📁 **e2e/** - Full-Stack HTTP Behavior
**Purpose:** Validate end-to-end behavior against running services

**Tests:**
- Deadline CRUD, validation & delivery
- Report generation & management
- Template CRUD & template-builder (config/generate/validate)
- Infra-facing checks (health, metrics, security, datasources, PDF security)

**Guarantees:** Real services behave correctly over HTTP

```bash
ORG_ID=your-org-id go test -tags=e2e ./tests/reporter/e2e/...
```

---

## Quick Start

### Run All Reporter Suites
Each suite is gated by its own build tag and has no aggregate `make` target; run them individually:
```bash
ORG_ID=your-org-id go test -tags=chaos       ./tests/reporter/chaos/...
ORG_ID=your-org-id go test -tags=fuzz        ./tests/reporter/fuzzy/...
ORG_ID=your-org-id go test -tags=property    ./tests/reporter/property/...
ORG_ID=your-org-id go test -tags=integration ./tests/reporter/integration/...
ORG_ID=your-org-id go test -tags=e2e         ./tests/reporter/e2e/...
```

### Configuration
Set `ORG_ID` or `X_ORGANIZATION_ID` environment variable:
```bash
export ORG_ID=06c4f684-19b0-449a-81f4-f9a4e503db83
go test -tags=chaos ./tests/reporter/chaos/...
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

| Category | Test/Fuzz funcs | Purpose |
|----------|-----------------|---------|
| **Chaos** | 39 | Infrastructure failures |
| **Fuzzy** | 24 | Input validation |
| **Property** | 70 | Invariants |
| **Integration** | varies | Component integration |
| **E2E** | varies | Full-stack HTTP behavior |

Counts are top-level `Test`/`Fuzz` functions (excluding `TestMain`); fuzz functions expand into many generated cases at runtime.

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
- Testing component interactions against real dependencies
- Validating repository/datasource behavior
- Verifying business logic

### Use **E2E** when:
- Validating full user workflows over HTTP
- Testing against running services
- Verifying end-to-end behavior

---

**All tests work together to ensure Reporter is secure, reliable, and resilient.**

