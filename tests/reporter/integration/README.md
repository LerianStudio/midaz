# Integration Tests

## Purpose
Validate end-to-end workflows and component interactions.

## What We Test
- **Report generation flow** - Template → Queue → Worker → Report
- **Template lifecycle** - Create, update, delete, list
- **Filter application** - Complex queries across datasources
- **Status transitions** - Processing → Finished/Error
- **File storage** - SeaweedFS integration
- **Database operations** - MongoDB persistence

## Key Guarantees
✅ **End-to-end workflows** - Complete user journeys work  
✅ **Component integration** - Manager ↔ Worker ↔ Queue ↔ Storage  
✅ **Real datasources** - Actual MongoDB, PostgreSQL queries  
✅ **Status accuracy** - Reports reflect actual state  

## Running
```bash
ORG_ID=your-org-id make test-integration
```

## Test Categories
- **Template CRUD** - Create, Read, Update, Delete operations
- **Report Generation** - Full generation pipeline
- **Filtering** - Advanced query operators
- **Error Handling** - Invalid inputs, missing resources
- **Concurrency** - Multiple reports simultaneously

