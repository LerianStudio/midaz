# Midaz Cloud Transformation Plan
## Infrastructure Decoupling for Hybrid/Cloud Deployment

**Objective**: Transform Midaz from direct database-coupled architecture to cloud-native, tenant-aware architecture enabling hybrid deployments.

**Timeline**: 6-9 months
**Priority**: CRITICAL (blocking Coreo Cloud/Hybrid vision)
**Owner**: Core Engineering Team

---

## 🎯 Why This Transformation is Critical

### **The Scale Imperative**
- **Current**: Serving dozens of enterprise customers at $20K/month ($240K ARR)
- **Vision**: Serving millions of developers globally ($1B+ ARR by year 5)
- **Constraint**: Cannot scale without multi-tenant cloud architecture

### **The Vercel Analogy Applied**
**Next.js → Vercel = Midaz → Coreo**

- **Next.js**: Open-source React framework that developers love
- **Midaz**: Open-source ledger that fintech developers love
- **Vercel**: Managed platform abstracting AWS complexity for frontend developers
- **Coreo**: Managed platform abstracting financial infrastructure complexity for fintech developers

### **Market Reality Check**
Without this transformation, Lerian remains:
- ❌ **Limited to enterprise-only** (complex self-hosted deployments)
- ❌ **No path to scale** (each customer requires dedicated infrastructure team)
- ❌ **Vulnerable to competitors** (Mambu, Thought Machine have cloud platforms)
- ❌ **Missing developer market** (millions of developers need simple fintech infrastructure)

With this transformation, Lerian becomes:
- ✅ **Global developer platform** (serve hobbyists to enterprises)
- ✅ **Magical developer experience** (git push → compliant fintech app)
- ✅ **Network effects** (plugin marketplace, community growth)
- ✅ **Platform economics** (usage-based pricing, ecosystem revenue)

### **Midaz's Role as the Foundation**
Midaz is the **core engine** that powers the entire Coreo platform:
- **For Developers**: The reliable, proven ledger they build upon
- **For Lerian**: The open-source funnel that drives Coreo adoption
- **For Ecosystem**: The standardized financial primitive that plugins extend

**Without cloud-native Midaz, there is no Coreo. Without Coreo, there is no path to $1B.**

---

## Phase 1: Data Access Layer Foundation (Weeks 1-8)

### Week 1-2: Architecture Design & Interfaces

- [ ] **Define Data Plane Client Interface**
  ```go
  type DataPlaneClient interface {
      ExecuteQuery(ctx context.Context, tenant TenantID, query Query) (Result, error)
      ExecuteTransaction(ctx context.Context, tenant TenantID, txn Transaction) error
      StreamEvents(ctx context.Context, tenant TenantID, eventType string) (<-chan Event, error)
      HealthCheck(ctx context.Context) error
  }
  ```

- [ ] **Design Tenant Context Propagation**
  ```go
  type TenantContext struct {
      TenantID     string
      OrgID        string
      Environment  string // dev/staging/prod
      Region       string // sa-east-1, us-east-1
      IsolationLevel IsolationLevel // schema, database, cluster
  }
  ```

- [ ] **Create Query Abstraction Types**
  ```go
  type Query struct {
      Operation string // SELECT, INSERT, UPDATE, DELETE
      Table     string
      Conditions map[string]interface{}
      TenantFilter TenantFilter
  }
  ```

- [ ] **Design Event Streaming Abstraction**
  ```go
  type EventStreamClient interface {
      Publish(ctx context.Context, tenant TenantID, event Event) error
      Subscribe(ctx context.Context, tenant TenantID, topics []string) (<-chan Event, error)
  }
  ```

### Week 3-4: Repository Interface Updates

- [ ] **Update Transaction Repository Interface**
  ```go
  // FROM: NewTransactionPostgreSQLRepository(postgresConnection)
  // TO: NewTransactionRepository(dataPlaneClient DataPlaneClient)

  type TransactionRepository interface {
      Create(ctx context.Context, tenant TenantID, txn *Transaction) error
      GetByID(ctx context.Context, tenant TenantID, id string) (*Transaction, error)
      List(ctx context.Context, tenant TenantID, filters TransactionFilters) ([]*Transaction, error)
      Update(ctx context.Context, tenant TenantID, id string, updates map[string]interface{}) error
  }
  ```

- [ ] **Update Operation Repository Interface**
  ```go
  type OperationRepository interface {
      Create(ctx context.Context, tenant TenantID, op *Operation) error
      GetByTransactionID(ctx context.Context, tenant TenantID, txnID string) ([]*Operation, error)
      UpdateStatus(ctx context.Context, tenant TenantID, id string, status OperationStatus) error
  }
  ```

- [ ] **Update Balance Repository Interface**
  ```go
  type BalanceRepository interface {
      Get(ctx context.Context, tenant TenantID, accountID string) (*Balance, error)
      Update(ctx context.Context, tenant TenantID, accountID string, amount Money) error
      CreateSnapshot(ctx context.Context, tenant TenantID, accountID string) error
  }
  ```

- [ ] **Update AssetRate Repository Interface**
  ```go
  type AssetRateRepository interface {
      Get(ctx context.Context, tenant TenantID, from, to Asset) (*AssetRate, error)
      Set(ctx context.Context, tenant TenantID, rate *AssetRate) error
      List(ctx context.Context, tenant TenantID, filters AssetRateFilters) ([]*AssetRate, error)
  }
  ```

### Week 5-6: Data Plane Service Implementation

- [ ] **Create Data Plane Service Package**
  ```bash
  mkdir -p components/data-plane-api/
  mkdir -p components/data-plane-api/internal/{domain,application,infrastructure}
  mkdir -p components/data-plane-api/cmd/app/
  ```

- [ ] **Implement PostgreSQL Data Plane Adapter**
  ```go
  type PostgreSQLDataPlane struct {
      primaryConn   *sql.DB
      replicaConn   *sql.DB
      tenantRouter  TenantRouter
      queryBuilder  QueryBuilder
  }

  func (p *PostgreSQLDataPlane) ExecuteQuery(ctx context.Context, tenant TenantID, query Query) (Result, error) {
      // Add tenant isolation to all queries
      // Route to appropriate database/schema
      // Apply row-level security
  }
  ```

- [ ] **Implement MongoDB Data Plane Adapter**
  ```go
  type MongoDataPlane struct {
      client       *mongo.Client
      tenantRouter TenantRouter
  }

  func (m *MongoDataPlane) ExecuteQuery(ctx context.Context, tenant TenantID, query Query) (Result, error) {
      // Add tenant filter to all MongoDB queries
      // Use tenant-specific collections/databases
  }
  ```

- [ ] **Implement Redis Data Plane Adapter**
  ```go
  type RedisDataPlane struct {
      client       redis.Client
      keyPrefixer  TenantKeyPrefixer
  }

  func (r *RedisDataPlane) ExecuteQuery(ctx context.Context, tenant TenantID, query Query) (Result, error) {
      // Prefix all Redis keys with tenant ID
      // Implement tenant-aware operations
  }
  ```

- [ ] **Create Tenant Router Implementation**
  ```go
  type TenantRouter interface {
      GetDatabaseConnection(tenant TenantID) (DatabaseConnection, error)
      GetSchemaName(tenant TenantID) (string, error)
      ValidateTenantAccess(ctx context.Context, tenant TenantID) error
  }
  ```

### Week 7-8: Repository Adapter Migration

- [ ] **Create New Repository Implementations**
  ```go
  // Replace direct PostgreSQL with data plane client
  type DataPlaneTransactionRepository struct {
      dataPlane DataPlaneClient
      mapper    TransactionMapper
  }

  func (r *DataPlaneTransactionRepository) Create(ctx context.Context, tenant TenantID, txn *Transaction) error {
      query := r.mapper.ToInsertQuery(txn)
      return r.dataPlane.ExecuteQuery(ctx, tenant, query)
  }
  ```

- [ ] **Update Bootstrap Configuration**
  ```go
  // FROM: Direct database connections
  // postgresConnection := &libPostgres.PostgresConnection{...}

  // TO: Data plane client
  dataPlaneClient := dataplane.NewClient(cfg.DataPlaneEndpoint, cfg.DataPlaneAuth)
  transactionRepo := repositories.NewDataPlaneTransactionRepository(dataPlaneClient)
  ```

- [ ] **Implement Backward Compatibility Mode**
  ```go
  // Support both deployment modes during transition
  if cfg.CloudDeployment {
      // Use data plane client
      dataPlaneClient := dataplane.NewClient(cfg.DataPlaneEndpoint)
      transactionRepo = repositories.NewDataPlaneTransactionRepository(dataPlaneClient)
  } else {
      // Use direct database (existing behavior)
      transactionRepo = repositories.NewPostgreSQLTransactionRepository(postgresConnection)
  }
  ```

---

## Phase 2: Event Streaming Decoupling (Weeks 9-12)

### Week 9-10: Event Abstraction Layer

- [ ] **Define Event Streaming Interface**
  ```go
  type EventStreamClient interface {
      PublishTransaction(ctx context.Context, tenant TenantID, event TransactionEvent) error
      PublishBalance(ctx context.Context, tenant TenantID, event BalanceEvent) error
      SubscribeToTransactions(ctx context.Context, tenant TenantID) (<-chan TransactionEvent, error)
      SubscribeToBalances(ctx context.Context, tenant TenantID) (<-chan BalanceEvent, error)
  }
  ```

- [ ] **Create Event Types with Tenant Context**
  ```go
  type TransactionEvent struct {
      TenantID      string
      TransactionID string
      EventType     string // created, updated, completed, failed
      Payload       Transaction
      Timestamp     time.Time
      TraceID       string
  }
  ```

- [ ] **Design Cross-Account Event Bridge**
  ```go
  type CrossAccountEventBridge struct {
      localBridge    EventBridge // RabbitMQ for same-account
      remoteBridge   EventBridge // EventBridge/SQS for cross-account
      tenantRouter   TenantRouter
  }
  ```

### Week 11-12: RabbitMQ Abstraction

- [ ] **Replace Direct RabbitMQ with Event Client**
  ```go
  // FROM: Direct RabbitMQ producer
  producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)

  // TO: Event stream client
  eventClient := eventstream.NewClient(cfg.EventStreamConfig)
  ```

- [ ] **Update Balance Event Publishing**
  ```go
  // Update balance creation to use tenant-aware events
  func (uc *UseCase) CreateBalance(ctx context.Context, tenant TenantID, balance *Balance) error {
      if err := uc.BalanceRepo.Create(ctx, tenant, balance); err != nil {
          return err
      }

      event := BalanceEvent{
          TenantID: string(tenant),
          BalanceID: balance.ID,
          EventType: "balance_created",
          Payload: balance,
      }
      return uc.EventClient.PublishBalance(ctx, tenant, event)
  }
  ```

---

## Phase 3: Tenant Context Integration (Weeks 13-16)

### Week 13-14: Middleware and Context Propagation

- [ ] **Create Tenant Middleware**
  ```go
  type TenantMiddleware struct {
      jwtValidator JWTValidator
      tenantExtractor TenantExtractor
  }

  func (m *TenantMiddleware) Handle(c *fiber.Ctx) error {
      tenant, err := m.tenantExtractor.ExtractFromJWT(c.Get("Authorization"))
      if err != nil {
          return err
      }
      c.Locals("tenant", tenant)
      return c.Next()
  }
  ```

- [ ] **Update HTTP Handlers with Tenant Context**
  ```go
  func (h *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
      tenant := c.Locals("tenant").(TenantID)
      var req CreateTransactionRequest
      if err := c.BodyParser(&req); err != nil {
          return err
      }

      // Pass tenant to use case
      result, err := h.Command.CreateTransaction(c.Context(), tenant, req.ToTransaction())
      if err != nil {
          return err
      }
      return c.JSON(result)
  }
  ```

- [ ] **Update Use Cases with Tenant Parameters**
  ```go
  // Update all use case methods to accept tenant
  func (uc *UseCase) CreateTransaction(ctx context.Context, tenant TenantID, txn *Transaction) (*Transaction, error) {
      // Validate tenant access
      if err := uc.TenantValidator.Validate(ctx, tenant); err != nil {
          return nil, err
      }

      // Use tenant in all repository calls
      return uc.TransactionRepo.Create(ctx, tenant, txn)
  }
  ```

### Week 15-16: Database Schema Tenant Isolation

- [ ] **Design Tenant Isolation Strategy**
  ```sql
  -- Option 1: Schema per tenant
  CREATE SCHEMA tenant_abc123;
  CREATE TABLE tenant_abc123.transactions (...);

  -- Option 2: Database per tenant (preferred for enterprise)
  CREATE DATABASE midaz_tenant_abc123;

  -- Option 3: Row-level security (for multi-tenant shared DB)
  ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;
  CREATE POLICY tenant_isolation ON transactions FOR ALL TO application_role
    USING (tenant_id = current_setting('app.current_tenant'));
  ```

- [ ] **Implement Tenant Database Provisioning**
  ```go
  type TenantProvisioner interface {
      ProvisionTenant(ctx context.Context, tenant TenantID, config TenantConfig) error
      DeprovisionTenant(ctx context.Context, tenant TenantID) error
      MigrateTenant(ctx context.Context, tenant TenantID, version string) error
  }
  ```

- [ ] **Update Migration Scripts for Multi-Tenancy**
  ```bash
  # Create tenant-aware migration tool
  ./midaz-migrate --tenant=abc123 --operation=up
  ./midaz-migrate --tenant=abc123 --operation=seed-data
  ```

---

## Phase 4: Cloud Deployment Readiness (Weeks 17-20)

### Week 17-18: Configuration Abstraction

- [ ] **Create Cloud-Native Configuration**
  ```go
  type Config struct {
      // Remove direct database configs
      // PrimaryDBHost, ReplicaDBHost, MongoURI, etc.

      // Add data plane config
      DataPlane DataPlaneConfig `env:"DATA_PLANE"`

      // Add deployment mode
      DeploymentMode DeploymentMode `env:"DEPLOYMENT_MODE"` // local, cloud, hybrid

      // Add tenant config
      TenantMode TenantMode `env:"TENANT_MODE"` // single, multi
  }

  type DataPlaneConfig struct {
      Endpoint     string `env:"DATA_PLANE_ENDPOINT"`
      AuthToken    string `env:"DATA_PLANE_AUTH_TOKEN"`
      TLSConfig    TLSConfig
      Timeout      time.Duration
      RetryPolicy  RetryPolicy
  }
  ```

- [ ] **Implement Configuration Factory Pattern**
  ```go
  func NewConfigForDeployment(mode DeploymentMode) (*Config, error) {
      switch mode {
      case DeploymentModeLocal:
          return newLocalConfig() // Direct DB connections
      case DeploymentModeCloud:
          return newCloudConfig() // Data plane client
      case DeploymentModeHybrid:
          return newHybridConfig() // Remote data plane
      }
  }
  ```

### Week 19-20: Service Containerization

- [ ] **Update Dockerfile for Multi-Mode Deployment**
  ```dockerfile
  # Support multiple deployment modes
  FROM golang:1.25-alpine AS builder
  COPY . .
  RUN go build -ldflags="-X main.deploymentMode=${DEPLOYMENT_MODE}" ./cmd/app

  FROM alpine:latest
  RUN apk --no-cache add ca-certificates
  COPY --from=builder /app/app /app/
  COPY configs/ /configs/
  CMD ["./app"]
  ```

- [ ] **Create Kubernetes Manifests**
  ```yaml
  # deployment-cloud.yaml
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: midaz-transaction-service
  spec:
    template:
      spec:
        containers:
        - name: transaction
          env:
          - name: DEPLOYMENT_MODE
            value: "cloud"
          - name: DATA_PLANE_ENDPOINT
            valueFrom:
              configMapKeyRef:
                name: midaz-config
                key: data-plane-endpoint
  ```

- [ ] **Create Helm Charts for Multi-Tenant Deployment**
  ```yaml
  # values-cloud.yaml
  deploymentMode: cloud
  tenantMode: multi
  dataPlane:
    endpoint: "data-plane-api.internal"
    authToken: "{{ .Values.dataPlane.authToken }}"
  ```

---

## Phase 5: Testing & Validation (Weeks 21-24)

### Week 21-22: Local Multi-Tenant Testing

- [ ] **Create Multi-Tenant Test Suite**
  ```go
  func TestMultiTenantIsolation(t *testing.T) {
      // Test that tenant A cannot access tenant B data
      // Test that queries are properly isolated
      // Test that events are tenant-scoped
  }
  ```

- [ ] **Create Data Plane Mock for Testing**
  ```go
  type MockDataPlaneClient struct {
      tenantData map[TenantID]map[string]interface{}
      events     map[TenantID][]Event
  }
  ```

- [ ] **Performance Benchmarks**
  ```bash
  # Benchmark direct DB vs data plane client
  go test -bench=BenchmarkDirectDB ./...
  go test -bench=BenchmarkDataPlaneClient ./...
  ```

### Week 23-24: Integration Testing

- [ ] **Deploy Test Environment with Data Plane**
  ```bash
  # Deploy data-plane-api service
  kubectl apply -f deployments/data-plane-api/

  # Deploy Midaz in cloud mode
  helm install midaz ./charts/midaz --set deploymentMode=cloud
  ```

- [ ] **End-to-End Multi-Tenant Tests**
  ```go
  func TestE2EMultiTenantTransactions(t *testing.T) {
      // Create transactions for multiple tenants
      // Verify isolation and proper routing
      // Test event streaming across tenants
  }
  ```

- [ ] **Load Testing with Multiple Tenants**
  ```bash
  # Test with 10 tenants, 1000 transactions each
  go run tests/load/multi-tenant-load.go --tenants=10 --transactions-per-tenant=1000
  ```

---

## Phase 6: Production Migration Strategy (Weeks 25-28)

### Week 25-26: Migration Tooling

- [ ] **Create Tenant Migration Tool**
  ```go
  type TenantMigrator struct {
      sourceDB     *sql.DB
      dataPlane    DataPlaneClient
      tenantConfig TenantConfig
  }

  func (m *TenantMigrator) MigrateTenant(ctx context.Context, tenant TenantID) error {
      // Extract existing tenant data
      // Transform to tenant-aware format
      // Load into new data plane
      // Verify data integrity
  }
  ```

- [ ] **Zero-Downtime Migration Strategy**
  ```bash
  # Migration phases
  1. Deploy data-plane-api alongside existing infrastructure
  2. Dual-write to both old and new systems
  3. Verify data consistency
  4. Switch reads to new system
  5. Stop dual-write, remove old system
  ```

### Week 27-28: Monitoring & Observability

- [ ] **Add Tenant-Aware Metrics**
  ```go
  // Prometheus metrics with tenant labels
  var (
      transactionsTotal = prometheus.NewCounterVec(
          prometheus.CounterOpts{Name: "midaz_transactions_total"},
          []string{"tenant_id", "operation", "status"},
      )
      responseTime = prometheus.NewHistogramVec(
          prometheus.HistogramOpts{Name: "midaz_response_time_seconds"},
          []string{"tenant_id", "endpoint"},
      )
  )
  ```

- [ ] **Implement Distributed Tracing**
  ```go
  func (h *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
      ctx, span := tracer.Start(c.Context(), "midaz.create_transaction")
      defer span.End()

      tenant := c.Locals("tenant").(TenantID)
      span.SetAttributes(
          attribute.String("tenant.id", string(tenant)),
          attribute.String("tenant.org", tenant.OrgID),
      )

      // Continue with business logic
  }
  ```

- [ ] **Add Health Checks for Data Plane**
  ```go
  func (h *HealthHandler) DataPlaneHealth(c *fiber.Ctx) error {
      tenant := c.Locals("tenant").(TenantID)
      if err := h.dataPlane.HealthCheck(c.Context()); err != nil {
          return c.Status(503).JSON(fiber.Map{"status": "unhealthy", "error": err.Error()})
      }
      return c.JSON(fiber.Map{"status": "healthy"})
  }
  ```

---

## Configuration Changes Required

### Current Config Structure (TO BE REPLACED):
```go
type Config struct {
    PrimaryDBHost     string `env:"DB_HOST"`
    PrimaryDBUser     string `env:"DB_USER"`
    PrimaryDBPassword string `env:"DB_PASSWORD"`
    MongoURI          string `env:"MONGO_URI"`
    RedisHost         string `env:"REDIS_HOST"`
    RabbitURI         string `env:"RABBITMQ_URI"`
    // ... direct infrastructure configs
}
```

### New Cloud-Ready Config Structure:
```go
type Config struct {
    DeploymentMode DeploymentMode    `env:"DEPLOYMENT_MODE"`
    DataPlane      DataPlaneConfig   `env:"DATA_PLANE"`
    EventStream    EventStreamConfig `env:"EVENT_STREAM"`
    Tenant         TenantConfig      `env:"TENANT"`

    // Keep for backward compatibility in local mode
    Legacy LegacyConfig `env:"LEGACY"`
}
```

---

## Testing Strategy

### Unit Tests
- [ ] **Repository Tests with Mock Data Plane**
- [ ] **Use Case Tests with Tenant Context**
- [ ] **Handler Tests with Tenant Middleware**

### Integration Tests
- [ ] **Multi-Tenant Data Isolation Tests**
- [ ] **Cross-Account Event Streaming Tests**
- [ ] **Performance Tests (Direct DB vs Data Plane)**

### End-to-End Tests
- [ ] **Full Transaction Flow with Multiple Tenants**
- [ ] **Event Processing Across Tenant Boundaries**
- [ ] **Disaster Recovery and Failover Tests**

---

## Risk Mitigation

### Performance Risks
- [ ] **Implement Caching Layer** between repositories and data plane
- [ ] **Connection Pooling** for data plane client
- [ ] **Circuit Breakers** for data plane failures

### Data Consistency Risks
- [ ] **Implement Dual-Write Strategy** during migration
- [ ] **Data Verification Tools** to ensure consistency
- [ ] **Rollback Procedures** if migration fails

### Security Risks
- [ ] **mTLS** for data plane communication
- [ ] **JWT-based** tenant authentication
- [ ] **Audit Logging** for all tenant operations

---

## Definition of Done

### Phase 1 Complete When:
- [ ] All repository interfaces accept tenant context
- [ ] Data plane client interfaces defined and documented
- [ ] Backward compatibility maintained for existing deployments
- [ ] Unit tests pass with new interfaces

### Phase 2 Complete When:
- [ ] Event streaming abstracted from direct RabbitMQ
- [ ] Cross-account event bridge implemented
- [ ] Event tests pass with tenant isolation

### Phase 3 Complete When:
- [ ] All HTTP endpoints extract and use tenant context
- [ ] Database queries include tenant isolation
- [ ] Multi-tenant integration tests pass

### Full Transformation Complete When:
- [ ] **Midaz can deploy in cloud mode** with remote data plane
- [ ] **Multi-tenant isolation verified** through comprehensive testing
- [ ] **Performance overhead <10%** compared to direct database access
- [ ] **Zero-downtime migration** from existing deployments proven
- [ ] **Hybrid deployment** successfully tested with cross-account setup

---

## Next Steps After Completion

1. **Deploy to Coreo Cloud** as managed service
2. **Enable Hybrid Deployments** with customer data planes
3. **Add Multi-Region Support** with data replication
4. **Implement Auto-Scaling** based on tenant load
5. **Add Advanced Tenant Features** (custom schemas, compliance rules)

This transformation enables Midaz to become the foundation for Coreo's "Vercel for fintech" vision.