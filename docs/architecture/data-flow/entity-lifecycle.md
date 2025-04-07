# Entity Lifecycle

**Navigation:** [Home](../../../) > [Architecture](../) > [Data Flow](./) > Entity Lifecycle

This document describes the lifecycle of entities in Midaz, from creation to deletion, and the flow of entity data through the system.

## Entity Hierarchy

Midaz implements a hierarchical entity model:

```
Organization
  │
  ├── Ledger
  │    │
  │    ├── Asset
  │    │
  │    ├── Segment
  │    │
  │    ├── Portfolio
  │    │
  │    └── Account
  │         │
  │         └── Balance
  │
  └── Organization (optional parent-child relationship)
```

Each entity has its own lifecycle, but follows consistent patterns across the system.

## Lifecycle Stages

### 1. Entity Creation

The creation process follows these steps:

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐     ┌──────────────┐     ┌────────────────┐
│  HTTP API    │     │ Command Layer │     │ Repository      │     │  Database    │     │  Event         │
│  Controller  │────►│ Create Entity │────►│ Implementation  │────►│  Persistence │────►│  Publication   │
└──────────────┘     └───────────────┘     └─────────────────┘     └──────────────┘     └────────────────┘
       ▲                     │                                                                  │
       │                     │                                                                  │
       │                     ▼                                                                  ▼
       │               ┌───────────────┐                                                ┌────────────────┐
       │               │   Validate    │                                                │ Event          │
       │               │   Constraints │                                                │ Consumers      │
       │               └───────────────┘                                                └────────────────┘
       │                                                                                       │
       │                                                                                       ▼
       │                                                                                ┌────────────────┐
       │                                                                                │ Related Entity │
       └────────────────────────────────────────────────────────────────────────────────┤ Processing     │
                                                                                        └────────────────┘
```

Key steps in entity creation:

1. **Request Validation**: Input data is validated against schema and business rules
2. **UUID Generation**: A unique identifier is assigned to the entity
3. **Status Assignment**: Initial status (typically "ACTIVE") is set
4. **Database Persistence**: Entity is stored in PostgreSQL
5. **Metadata Storage**: Any metadata is stored in MongoDB
6. **Event Publication**: Entity creation event is published to RabbitMQ
7. **Response**: Entity data with ID is returned to the caller

Example code flow for entity creation:

```go
// HTTP Controller
func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
    // Parse request
    var request dto.OrganizationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputils.WriteError(w, errors.NewBadRequestError("invalid request body"))
        return
    }
    
    // Map to domain model
    organization := &mmodel.Organization{
        Name:          request.Name,
        LegalName:     request.LegalName,
        LegalDocument: request.LegalDocument,
        // ...other fields
    }
    
    // Call domain service
    result, err := c.createOrgUseCase.Execute(r.Context(), organization)
    if err != nil {
        httputils.WriteError(w, err)
        return
    }
    
    // Return response
    httputils.WriteJSON(w, http.StatusCreated, result)
}

// Command handler
func (u *CreateOrganizationUseCase) Execute(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
    // Validate
    if err := u.validateOrganization(organization); err != nil {
        return nil, err
    }
    
    // Generate UUID if not provided
    if organization.ID == uuid.Nil {
        organization.ID = uuid.New()
    }
    
    // Set default values
    organization.Status = "ACTIVE"
    organization.CreatedAt = time.Now()
    organization.UpdatedAt = time.Now()
    
    // Store in repository
    result, err := u.repository.Create(ctx, organization)
    if err != nil {
        return nil, err
    }
    
    // Store metadata if any
    if organization.Metadata != nil {
        err = u.metadataRepo.Store(ctx, "organization", result.ID, organization.Metadata)
        if err != nil {
            return nil, err
        }
    }
    
    // Publish event
    err = u.eventPublisher.PublishOrganizationCreated(ctx, result)
    if err != nil {
        log.Warn(ctx, "Failed to publish organization created event", err)
        // Continue despite event publishing failure
    }
    
    return result, nil
}
```

### 2. Entity Retrieval

Entities can be retrieved individually or as collections:

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐     ┌──────────────┐
│  HTTP API    │     │  Query Layer  │     │  Repository     │     │  Database    │
│  Controller  │────►│  Get Entity   │────►│  Implementation │────►│  Fetch       │
└──────────────┘     └───────────────┘     └──────────────┬──┘     └──────────────┘
       ▲                                                   │               │
       │                                                   │               │
       │                                                   ▼               │
       │                                           ┌─────────────────┐     │
       │                                           │  Metadata       │◄────┘
       │                                           │  Repository     │
       │                                           └────────┬────────┘
       │                                                    │
       │                                                    ▼
       │                                           ┌─────────────────┐
       └───────────────────────────────────────────┤  Combine Data  │
                                                   └─────────────────┘
```

Key aspects of entity retrieval:

1. **Query Parameters**: Filter, sort, and pagination parameters
2. **Repository Abstraction**: Data access through repository interfaces
3. **Metadata Combination**: Entity data combined with metadata
4. **Parent-Child Relationships**: Related entities retrieved as needed
5. **Soft Delete Handling**: Filtered to exclude deleted entities by default

Example code flow for entity retrieval:

```go
// Query handler
func (u *GetOrganizationUseCase) Execute(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
    // Fetch from repository
    organization, err := u.repository.Find(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Fetch metadata
    metadata, err := u.metadataRepo.Get(ctx, "organization", id)
    if err != nil && !errors.IsNotFound(err) {
        return nil, err
    }
    
    // Combine entity with metadata
    if metadata != nil {
        organization.Metadata = metadata
    }
    
    return organization, nil
}

// Repository implementation
func (r *PostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
    query := `SELECT id, name, legal_name, legal_document, status, created_at, updated_at 
              FROM organizations 
              WHERE id = $1 AND deleted_at IS NULL`
    
    var org mmodel.Organization
    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &org.ID,
        &org.Name,
        &org.LegalName,
        &org.LegalDocument,
        &org.Status,
        &org.CreatedAt,
        &org.UpdatedAt,
    )
    
    if err == sql.ErrNoRows {
        return nil, errors.NewNotFoundError(fmt.Sprintf("organization with id %s not found", id))
    }
    
    if err != nil {
        return nil, errors.FromError(err).WithMessage("error finding organization")
    }
    
    return &org, nil
}
```

### 3. Entity Update

Updates follow a similar pattern to creation:

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐     ┌──────────────┐     ┌────────────────┐
│  HTTP API    │     │ Command Layer │     │ Repository      │     │  Database    │     │  Event         │
│  Controller  │────►│ Update Entity │────►│ Implementation  │────►│  Update      │────►│  Publication   │
└──────────────┘     └───────────────┘     └─────────────────┘     └──────────────┘     └────────────────┘
       ▲                     │                                                                  │
       │                     │                                                                  │
       │                     ▼                                                                  ▼
       │               ┌───────────────┐                                                ┌────────────────┐
       │               │   Validate    │                                                │ Event          │
       │               │   Changes     │                                                │ Consumers      │
       │               └───────────────┘                                                └────────────────┘
       │                                                                                       │
       │                                                                                       ▼
       │                                                                                ┌────────────────┐
       │                                                                                │ Related Entity │
       └────────────────────────────────────────────────────────────────────────────────┤ Updates        │
                                                                                        └────────────────┘
```

Key aspects of entity updates:

1. **Partial Updates**: Only specified fields are updated
2. **Optimistic Concurrency**: Version checking (in some entities)
3. **Validation**: Business rules enforced on updated fields
4. **Metadata Updates**: Separate from entity updates
5. **Event Publication**: Update events for downstream processing

Example code flow for entity update:

```go
// Update handler
func (u *UpdateOrganizationUseCase) Execute(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error) {
    // Check if entity exists
    existing, err := u.repository.Find(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Apply partial updates
    if organization.Name != "" {
        existing.Name = organization.Name
    }
    if organization.LegalName != "" {
        existing.LegalName = organization.LegalName
    }
    if organization.Status != "" {
        existing.Status = organization.Status
    }
    
    // Set updated timestamp
    existing.UpdatedAt = time.Now()
    
    // Update in repository
    result, err := u.repository.Update(ctx, id, existing)
    if err != nil {
        return nil, err
    }
    
    // Update metadata if provided
    if organization.Metadata != nil {
        err = u.metadataRepo.Update(ctx, "organization", id, organization.Metadata)
        if err != nil {
            return nil, err
        }
    }
    
    // Publish event
    err = u.eventPublisher.PublishOrganizationUpdated(ctx, result)
    if err != nil {
        log.Warn(ctx, "Failed to publish organization updated event", err)
    }
    
    return result, nil
}
```

### 4. Entity Deletion

Entities in Midaz are soft-deleted:

```
┌──────────────┐     ┌───────────────┐     ┌─────────────────┐     ┌──────────────┐     ┌────────────────┐
│  HTTP API    │     │ Command Layer │     │ Repository      │     │  Database    │     │  Event         │
│  Controller  │────►│ Delete Entity │────►│ Implementation  │────►│  Soft Delete │────►│  Publication   │
└──────────────┘     └───────────────┘     └─────────────────┘     └──────────────┘     └────────────────┘
       ▲                     │                                                                  │
       │                     │                                                                  │
       │                     ▼                                                                  ▼
       │               ┌───────────────┐                                                ┌────────────────┐
       │               │   Validate    │                                                │ Event          │
       │               │ Delete Allowed│                                                │ Consumers      │
       │               └───────────────┘                                                └────────────────┘
       │                                                                                       │
       │                                                                                       ▼
       │                                                                                ┌────────────────┐
       │                                                                                │ Related Entity │
       └────────────────────────────────────────────────────────────────────────────────┤ Cascades       │
                                                                                        └────────────────┘
```

Key aspects of entity deletion:

1. **Soft Delete**: Entities are marked as deleted with a timestamp rather than physically removed
2. **Cascading Deletes**: Parent-child relationships may trigger cascading soft-deletes
3. **Validation**: Business rules may prevent deletion in certain states
4. **Event Publication**: Deletion events for downstream processing

Example code flow for entity deletion:

```go
// Delete handler
func (u *DeleteOrganizationUseCase) Execute(ctx context.Context, id uuid.UUID) error {
    // Check if entity exists
    _, err := u.repository.Find(ctx, id)
    if err != nil {
        return err
    }
    
    // Check business rules (e.g., no active children)
    count, err := u.ledgerRepo.CountByOrganizationID(ctx, id)
    if err != nil {
        return err
    }
    
    if count > 0 {
        return errors.NewConflictError("cannot delete organization with active ledgers")
    }
    
    // Perform soft delete
    err = u.repository.Delete(ctx, id)
    if err != nil {
        return err
    }
    
    // Publish event
    err = u.eventPublisher.PublishOrganizationDeleted(ctx, id)
    if err != nil {
        log.Warn(ctx, "Failed to publish organization deleted event", err)
    }
    
    return nil
}

// Repository implementation
func (r *PostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
    query := `UPDATE organizations SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`
    
    result, err := r.db.ExecContext(ctx, query, time.Now(), id)
    if err != nil {
        return errors.FromError(err).WithMessage("error deleting organization")
    }
    
    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return errors.FromError(err).WithMessage("error getting rows affected")
    }
    
    if rowsAffected == 0 {
        return errors.NewNotFoundError(fmt.Sprintf("organization with id %s not found", id))
    }
    
    return nil
}
```

## Entity Validation

Entities go through several validation layers:

1. **Input Validation**: Basic field validation (required fields, formats)
2. **Business Rules**: Domain-specific rules (e.g., status transitions)
3. **Referential Integrity**: Foreign key constraints
4. **Uniqueness Constraints**: Prevent duplicates (e.g., unique names within parent)

Example validation logic:

```go
func (u *CreateOrganizationUseCase) validateOrganization(org *mmodel.Organization) error {
    // Required fields
    if org.Name == "" {
        return errors.NewValidationError("name is required")
    }
    if org.LegalName == "" {
        return errors.NewValidationError("legal name is required")
    }
    if org.LegalDocument == "" {
        return errors.NewValidationError("legal document is required")
    }
    
    // Business rules
    if org.ParentID != nil {
        // Check if parent exists
        _, err := u.repository.Find(ctx, *org.ParentID)
        if err != nil {
            return errors.NewValidationError("parent organization not found")
        }
    }
    
    // Check for duplicate names
    exists, err := u.repository.ExistsByName(ctx, org.Name)
    if err != nil {
        return err
    }
    if exists {
        return errors.NewConflictError("organization with this name already exists")
    }
    
    return nil
}
```

## Entity Relationships

Entities in Midaz have various types of relationships:

1. **Hierarchical Relationships**:
   - Organization can have parent/child Organizations
   - Account can have parent/child Accounts

2. **Ownership Relationships**:
   - Organization owns Ledgers
   - Ledger owns Assets, Segments, Portfolios, and Accounts
   - Portfolio contains Accounts

3. **Reference Relationships**:
   - Account references Asset (via assetCode)
   - Account may belong to Portfolio and/or Segment

## Metadata Management

Entities can have flexible metadata stored separately:

1. **MongoDB Storage**: Metadata stored in MongoDB collections
2. **Key-Value Structure**: Metadata stored as key-value pairs
3. **Entity Reference**: Linked to entity by ID and type
4. **Independent Lifecycle**: Metadata can be updated separately

Example metadata operations:

```go
// Store metadata
func (r *MetadataMongoDBRepository) Store(ctx context.Context, entityType string, entityID uuid.UUID, metadata map[string]interface{}) error {
    collection := r.db.Collection("metadata")
    
    doc := bson.M{
        "entity_type": entityType,
        "entity_id":   entityID.String(),
        "metadata":    metadata,
        "created_at":  time.Now(),
        "updated_at":  time.Now(),
    }
    
    _, err := collection.InsertOne(ctx, doc)
    if err != nil {
        return errors.FromError(err).WithMessage("error storing metadata")
    }
    
    return nil
}

// Get metadata
func (r *MetadataMongoDBRepository) Get(ctx context.Context, entityType string, entityID uuid.UUID) (map[string]interface{}, error) {
    collection := r.db.Collection("metadata")
    
    filter := bson.M{
        "entity_type": entityType,
        "entity_id":   entityID.String(),
    }
    
    var result struct {
        Metadata map[string]interface{} `bson:"metadata"`
    }
    
    err := collection.FindOne(ctx, filter).Decode(&result)
    if err == mongo.ErrNoDocuments {
        return nil, errors.NewNotFoundError("metadata not found")
    }
    if err != nil {
        return nil, errors.FromError(err).WithMessage("error getting metadata")
    }
    
    return result.Metadata, nil
}
```

## Event Publication

Entity lifecycle events are published to allow other components to react:

1. **Event Types**: Created, Updated, Deleted events
2. **Event Structure**: Contains entity ID, organization ID, and relevant data
3. **Message Broker**: RabbitMQ for event distribution
4. **Consumer Processing**: Services consume events to update related entities

Example event publication:

```go
// Publish account created event
func (p *ProducerRabbitMQ) PublishAccountCreated(ctx context.Context, account *mmodel.Account) error {
    // Create queue data
    queueData := &mmodel.QueueData{
        OrganizationID: account.OrganizationID,
        LedgerID:       account.LedgerID,
        AccountID:      account.ID,
        Payload: map[string]interface{}{
            "account_id":   account.ID.String(),
            "asset_code":   account.AssetCode,
            "account_type": account.Type,
            "status":       account.Status,
        },
    }
    
    // Create header ID for tracing
    headerID := uuid.New().String()
    
    // Publish to queue
    err := p.publishToQueue(ctx, "account.create", headerID, queueData)
    if err != nil {
        return errors.FromError(err).WithMessage("error publishing account created event")
    }
    
    return nil
}
```

## Related Entity Processing

When an entity is created or updated, related entities may need processing:

1. **Balance Creation**: When an Account is created, a Balance is automatically created
2. **Transaction Processing**: When a Transaction is created, Balances are updated
3. **Cascading Updates**: Status changes may propagate to child entities

Example of related entity processing:

```go
// Handle account created event
func (h *AccountCreatedHandler) Handle(ctx context.Context, data *mmodel.QueueData) error {
    // Extract account data
    accountID, err := uuid.Parse(data.Payload["account_id"].(string))
    if err != nil {
        return errors.FromError(err).WithMessage("invalid account ID")
    }
    
    assetCode := data.Payload["asset_code"].(string)
    accountType := data.Payload["account_type"].(string)
    
    // Create balance for account
    balance := &mmodel.Balance{
        ID:          uuid.New(),
        AccountID:   accountID,
        AssetCode:   assetCode,
        AccountType: accountType,
        Available:   "0",
        OnHold:      "0",
        Scale:       2,  // Default scale
        Version:     1,  // Initial version
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }
    
    // Store balance
    _, err = h.balanceRepo.Create(ctx, balance)
    if err != nil {
        return err
    }
    
    return nil
}
```

## Entity Lifecycle Benefits

The entity lifecycle management in Midaz provides several benefits:

1. **Consistency**: Standardized patterns across all entities
2. **Auditability**: Complete history through soft deletes and timestamps
3. **Loose Coupling**: Services can evolve independently
4. **Extensibility**: Metadata support for extending entities
5. **Resilience**: Event-driven architecture allows for retry mechanisms

## Next Steps

- [Transaction Lifecycle](./transaction-lifecycle.md) - How transactions flow through the system
- [Component Integration](../component-integration.md) - How components interact
- [Domain Models](../../domain-models/entity-hierarchy.md) - Learn about the entity models