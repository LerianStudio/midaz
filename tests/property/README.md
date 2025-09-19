Property-based tests check invariants across a wide input space. Replace scaffolds with domain properties, e.g.:

- Conservation of value across operations when routes balance.
- Idempotency of POSTs with idempotency keys.
- Pagination properties (no duplicates, stable ordering).

