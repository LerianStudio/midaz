# Integration Tests

## Purpose
Validate end-to-end workflows and component interactions.

## What We Test
- **Report repository/handler behavior** - Create, get-by-id, list with filters, count (full Template → Queue → Worker → Report pipeline lives in the e2e suite)
- **Template lifecycle** - Create, update, delete, list
- **Filter application** - Datasource cache behavior; corrupt/invalid filter keys do not corrupt maps
- **Status transitions** - Processing → Finished/Error
- **File storage** - SeaweedFS integration
- **Database operations** - MongoDB persistence

## Key Guarantees
✅ **End-to-end workflows** - Complete user journeys work  
✅ **Component integration** - Manager ↔ Worker ↔ Queue ↔ Storage  
✅ **Real datasources** - MongoDB-backed datasource cache/corruption behavior (live PostgreSQL datasource queries are covered in the e2e suite)  
✅ **Status accuracy** - Reports reflect actual state  

## Running
```bash
go test -tags integration ./tests/reporter/integration/...
```

> Note: `make test-integration` only discovers `*_integration_test.go` files
> under `./components` and `./pkg`; it does not scan `./tests`, so it does NOT
> run this reporter suite. Invoke `go test` directly as shown above.

## Test Categories
- **Template CRUD** - Create, Read, Update, Delete operations
- **Report Repository** - Create, get-by-id, list with filters, count
- **Filtering** - Datasource cache and filter-key handling
- **Error Handling** - Invalid inputs, missing resources

