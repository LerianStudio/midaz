# itestkit

**itestkit** is an integration and E2E testing framework for Go, focused on **real infrastructure**, **isolation**, **reproducibility**, and **chaos engineering**.

## Features

- **Real Infrastructure** — Spin up actual containers (PostgreSQL, MySQL, MongoDB, SQL Server, Oracle, Redis, RabbitMQ, SeaweedFS) deterministically
- **Chaos Engineering** — Built-in fault injection via [Toxiproxy](https://github.com/Shopify/toxiproxy) (latency, connection drops, timeouts)
- **Technology Agnostic** — Framework doesn't know about specific technologies; everything is a generic container
- **Fluent API** — Clean, composable builder pattern for test environment setup
- **E2E Support** — Run your application in a container with automatic healthchecks and environment rewriting

## Quick Start

```go
package myapp_test

import (
    "context"
    "testing"

    "github.com/your-org/itestkit"
    "github.com/your-org/itestkit/infra/postgres"
)

func TestWithPostgres(t *testing.T) {
    ctx := context.Background()

    // Create infrastructure
    pgInfra := postgres.NewPostgresInfra(postgres.PostgresConfig{
        Database: "testdb",
        Username: "testuser",
        Password: "testpass",
    })

    // Build the test suite
    suite, err := itestkit.New(t).
        WithInfra(pgInfra).
        Build(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer suite.Terminate(ctx)

    // Get connection string
    dsn, _ := pgInfra.DSN()

    // Use dsn in your tests...
}
```

## Architecture

```
itestkit
├── Builder         → Fluent API for configuring the test environment
├── Suite           → Running test environment
├── Env             → Endpoints and resources available to tests
├── Infra           → Common interface for any infrastructure
├── ChaosProvider   → Chaos abstraction (Toxiproxy or noop)
├── infra/          → Pre-built infrastructure (optional)
│   ├── postgres    → PostgreSQL
│   ├── mysql       → MySQL
│   ├── mongodb     → MongoDB
│   ├── redis       → Redis
│   ├── rabbitmq    → RabbitMQ
│   ├── mssql       → Microsoft SQL Server
│   ├── oracle      → Oracle Database
│   └── seaweedfs   → SeaweedFS (distributed file system)
└── addons/
    ├── e2ekit      → End-to-end testing addon
    ├── metricskit  → Chaos metrics and assertions
    └── queuekit    → Message queue testing with generics
```

## Core Concepts

### Builder

The `Builder` describes your test environment using a fluent API. Nothing executes until you call `Build()`.

```go
suite, err := itestkit.New(t).
    WithChaos(itestkit.ChaosConfig{Enabled: true}).
    WithInfra(pgInfra).
    WithInfra(rabbitInfra).
    Build(ctx)
```

### Suite

The `Suite` represents a running test environment. Always defer its termination:

```go
defer suite.Terminate(ctx)
```

### Env

The `Env` is the contract between the framework and your tests:

```go
// Access container endpoints
container := suite.Env().Containers["postgres"]
host := container.Host
port := container.Ports["5432/tcp"]

// Access chaos provider
suite.Chaos().AddLatency(ctx, "proxy-name", 500*time.Millisecond, 50*time.Millisecond)
```

### Infra Interface

All infrastructure implements the `Infra` interface:

```go
type Infra interface {
    Start(ctx context.Context, env *Env) error
    Terminate(ctx context.Context) error
}

// Optional: enables duplicate validation
type NamedInfra interface {
    Infra
    InfraKind() string
    InfraName() string
}
```

## Pre-built Infrastructure

The framework provides optional pre-built infrastructure in sub-packages. The core remains agnostic.

### PostgreSQL

```go
import "github.com/your-org/itestkit/infra/postgres"

pgInfra := postgres.NewPostgresInfra(postgres.PostgresConfig{
    Name:        "mydb",           // optional, defaults to "postgres"
    Image:       "postgres:16",    // optional, defaults to "postgres:16-alpine"
    Database:    "testdb",
    Username:    "user",
    Password:    "pass",
    EnableProxy: true,             // enable chaos proxy
})

suite, _ := itestkit.New(t).WithInfra(pgInfra).Build(ctx)

dsn, _ := pgInfra.DSN()
// postgres://user:pass@localhost:32780/testdb?sslmode=disable
```

### RabbitMQ

```go
import "github.com/your-org/itestkit/infra/rabbitmq"

rmqInfra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
    Name:        "broker",
    Image:       "rabbitmq:3.13-management-alpine",
    Username:    "guest",
    Password:    "guest",
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(rmqInfra).Build(ctx)

amqpURL, _ := rmqInfra.AMQPURL()
// amqp://guest:guest@localhost:32781/
```

#### RabbitMQ with Definitions File

Pre-configure exchanges, queues, bindings, and users using a `definitions.json` file:

```go
rmqInfra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
    Name:     "broker",
    Username: "admin",
    Password: "admin",
    Options: []rabbitmq.RabbitOption{
        rabbitmq.WithRabbitDefinitions("testdata/definitions.json"),
    },
})

suite, _ := itestkit.New(t).WithInfra(rmqInfra).Build(ctx)
// Exchanges, queues, and bindings are already configured
```

**definitions.json structure:**

```json
{
  "users": [
    {
      "name": "admin",
      "password_hash": "<hashed_password>",
      "hashing_algorithm": "rabbit_password_hashing_sha256",
      "tags": "administrator"
    }
  ],
  "vhosts": [
    { "name": "/" }
  ],
  "permissions": [
    {
      "user": "admin",
      "vhost": "/",
      "configure": ".*",
      "write": ".*",
      "read": ".*"
    }
  ],
  "exchanges": [
    { "name": "events", "vhost": "/", "type": "topic", "durable": true },
    { "name": "dlx", "vhost": "/", "type": "direct", "durable": true }
  ],
  "queues": [
    {
      "name": "jobs",
      "vhost": "/",
      "durable": true,
      "arguments": {
        "x-dead-letter-exchange": "dlx",
        "x-dead-letter-routing-key": "jobs.dlq"
      }
    },
    {
      "name": "jobs.dlq",
      "vhost": "/",
      "durable": true,
      "arguments": { "x-message-ttl": 86400000 }
    }
  ],
  "bindings": [
    {
      "source": "events",
      "vhost": "/",
      "destination": "jobs",
      "destination_type": "queue",
      "routing_key": "job.#"
    }
  ]
}
```

**Configuration considerations:**

| Field | Description |
|-------|-------------|
| `users` | Define users with password hashes (use `rabbitmqctl hash_password` to generate) |
| `vhosts` | Virtual hosts must be declared before permissions |
| `permissions` | User permissions per vhost (configure/write/read patterns) |
| `exchanges` | Types: `direct`, `topic`, `fanout`, `headers` |
| `queues` | Can include arguments for DLX, TTL, max-length, etc. |
| `bindings` | Connect exchanges to queues with routing keys |

**Generating password hashes:**

```bash
# Using rabbitmqctl (requires RabbitMQ installed)
rabbitmqctl hash_password "your_password"

# Or generate programmatically (SHA256 with 4-byte salt, base64 encoded)
```

**Important:** The `Username` and `Password` in `RabbitConfig` must match a user defined in the definitions file.

### Redis

```go
import "github.com/your-org/itestkit/infra/redis"

redisInfra := redis.NewRedisInfra(redis.RedisConfig{
    Name:        "cache",
    Password:    "secret",      // optional, no auth by default
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(redisInfra).Build(ctx)

addr, _ := redisInfra.Addr()  // "localhost:32782"
url, _ := redisInfra.URL()    // "redis://:secret@localhost:32782"
```

### MySQL

```go
import "github.com/your-org/itestkit/infra/mysql"

mysqlInfra := mysql.NewMySQLInfra(mysql.MySQLConfig{
    Name:        "mydb",
    Database:    "testdb",      // defaults to "testdb"
    Username:    "testuser",    // defaults to "testuser"
    Password:    "testpass",    // defaults to "testpass"
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(mysqlInfra).Build(ctx)

dsn, _ := mysqlInfra.DSN()
// testuser:testpass@tcp(localhost:32783)/testdb?parseTime=true
```

### MongoDB

```go
import "github.com/your-org/itestkit/infra/mongodb"

mongoInfra := mongodb.NewMongoDBInfra(mongodb.MongoDBConfig{
    Name:        "docstore",
    Username:    "admin",       // optional, no auth by default
    Password:    "secret",      // optional
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(mongoInfra).Build(ctx)

uri, _ := mongoInfra.URI()
// mongodb://admin:secret@localhost:32784 (with auth)
// mongodb://localhost:32784 (without auth)
```

### Microsoft SQL Server

```go
import "github.com/your-org/itestkit/infra/mssql"

mssqlInfra := mssql.NewMSSQLInfra(mssql.MSSQLConfig{
    Name:        "sqlserver",
    Database:    "master",                  // optional
    Password:    "YourStrong@Passw0rd",     // defaults to this
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(mssqlInfra).Build(ctx)

dsn, _ := mssqlInfra.DSN()
// sqlserver://sa:YourStrong@Passw0rd@localhost:32785?database=master
```

### Oracle Database

```go
import "github.com/your-org/itestkit/infra/oracle"

oracleInfra := oracle.NewOracleInfra(oracle.OracleConfig{
    Name:        "oradb",
    Password:    "testpass",    // defaults to "testpass"
    SID:         "XE",          // defaults to "XE"
    EnableProxy: true,
})

suite, _ := itestkit.New(t).WithInfra(oracleInfra).Build(ctx)

dsn, _ := oracleInfra.DSN()           // oracle://system:testpass@localhost:32786/XE
godrorDSN, _ := oracleInfra.GoDRORDSN() // system/testpass@localhost:32786/XE
```

**Note:** Oracle containers are slow to start (up to 10 minutes). Use longer timeouts in tests.

### SeaweedFS

```go
import "github.com/your-org/itestkit/infra/seaweedfs"

seaweedInfra := seaweedfs.NewSeaweedFSInfra(seaweedfs.SeaweedFSConfig{
    Name:  "storage",
    Image: "chrislusf/seaweedfs:latest",  // optional
})

suite, _ := itestkit.New(t).WithInfra(seaweedInfra).Build(ctx)

url, _ := seaweedInfra.URL()              // http://localhost:32787
host, port, _ := seaweedInfra.HostPort()  // "172.17.0.1", 32787
```

**Note:** SeaweedFS starts a cluster with Master, Volume, and Filer components. The `URL()` and `HostPort()` methods return the Filer endpoint.

## Generic Containers

Use `WithContainerCustomize` to spin up any container without the framework knowing about the technology:

```go
suite, _ := itestkit.New(t).
    WithContainerCustomize(itestkit.ContainerSpec{
        Name:         "elasticsearch",
        Image:        "elasticsearch:8.12.0",
        ExposedPorts: []string{"9200/tcp"},
        EnableProxy:  true,
        ProxyPrefix:  "es",
        Wait: itestkit.WaitListeningPort{
            Port: "9200/tcp",
        },
        Customizers: itestkit.CAll(
            itestkit.CEnv("discovery.type", "single-node"),
            itestkit.CEnv("xpack.security.enabled", "false"),
        ),
    }).
    Build(ctx)

// Access the container
es := suite.Env().Containers["elasticsearch"]
addr := es.Upstreams["9200/tcp"] // "localhost:32782"
```

## Customizers

Customizers are the primary extension mechanism, wrapping `testcontainers.ContainerCustomizer`:

| Customizer | Description |
|------------|-------------|
| `CEnv(key, value)` | Set environment variable |
| `CEnvs(map[string]string)` | Set multiple environment variables |
| `CImage(image)` | Override container image |
| `CCmd(args...)` | Set container command |
| `CExposedPorts(ports...)` | Expose additional ports |
| `CInitScriptDirEntryPoint(src, dest, mode)` | Copy init script |
| `CHostDockerInternal()` | Add host.docker.internal mapping |
| `CNetworks(networks...)` | Attach to networks |
| `CNetworkAliases(network, aliases...)` | Set network aliases |
| `CBindMount(host, container)` | Bind mount a volume |
| `CAll(customizers...)` | Combine multiple customizers |

## Chaos Engineering

Chaos is a first-class citizen in itestkit. When enabled, [Toxiproxy](https://github.com/Shopify/toxiproxy) starts automatically.

### Enable Chaos

```go
suite, _ := itestkit.New(t).
    WithChaos(itestkit.ChaosConfig{Enabled: true}).
    WithInfra(pgInfra).
    Build(ctx)
```

### Inject Faults

```go
// Add 500ms latency with 50ms jitter
suite.Chaos().AddLatency(ctx, "pg-postgres-5432", 500*time.Millisecond, 50*time.Millisecond)

// Add timeout - stop data flow and close after 5s
suite.Chaos().AddTimeout(ctx, "pg-postgres-5432", 5*time.Second)

// Limit bandwidth to 128 KB/s (simulate slow network)
suite.Chaos().AddBandwidth(ctx, "pg-postgres-5432", 128)

// Cut the connection (immediate timeout)
suite.Chaos().CutConnection(ctx, "pg-postgres-5432")

// Remove specific toxic
suite.Chaos().RemoveToxic(ctx, "pg-postgres-5432", "timeout")

// Remove all toxics
suite.Chaos().RemoveAllToxics(ctx, "pg-postgres-5432")
```

### Available Chaos Methods

| Method | Description | Parameters |
|--------|-------------|------------|
| `AddLatency(ctx, proxy, latency, jitter)` | Add network delay | latency: base delay, jitter: variance |
| `AddTimeout(ctx, proxy, timeout)` | Stop data flow, close after timeout | timeout: duration (0 = never close) |
| `AddBandwidth(ctx, proxy, rateKBps)` | Limit bandwidth | rateKBps: KB/s (e.g., 128 = slow DSL) |
| `CutConnection(ctx, proxy)` | Immediately timeout connection | - |
| `RemoveToxic(ctx, proxy, toxicName)` | Remove specific toxic | toxicName: "latency", "timeout", "bandwidth" |
| `RemoveAllToxics(ctx, proxy)` | Remove all toxics from proxy | - |

### Common Bandwidth Values

| Value (KB/s) | Simulates |
|--------------|-----------|
| 56 | Dial-up modem |
| 128 | Slow DSL |
| 512 | Basic broadband |
| 1024 | 1 Mbps connection |
| 10240 | 10 Mbps connection |

### Proxy Naming Convention

Proxy names follow the pattern: `<prefix>-<container-name>-<port>`

Examples:
- `pg-postgres-5432`
- `amqp-rabbitmq-5672`
- `redis-cache-6379`

## E2E Testing with e2ekit

The `e2ekit` addon runs your application inside a container, enabling true end-to-end tests.

```go
import "github.com/your-org/itestkit/addons/e2ekit"

// Start your application container
app, err := e2ekit.New(t).
    WithImage("my-api:latest").
    ExposePort(8080).
    WithEnvVar("DATABASE_URL", pgInfra.DSN()).
    WithWait(e2ekit.WaitHTTP(8080, "/health", 30*time.Second)).
    Run()
if err != nil {
    t.Fatal(err)
}
defer app.Terminate(ctx)

// Make requests to your app
resp, _ := http.Get(app.BaseURL + "/api/users")
```

### Building with Dockerfile

Build images directly from Dockerfiles:

```go
app, err := e2ekit.New(t).
    WithDockerfile(e2ekit.BuildConfig{
        ContextDir: "../../components/api",
        Dockerfile: "Dockerfile",
        BuildArgs: map[string]*string{
            "GO_VERSION": ptr("1.21"),
        },
    }).
    ExposePort(8080).
    WithWait(e2ekit.WaitHTTP(8080, "/health", 60*time.Second)).
    Run()
```

### Building with BuildKit Secrets

For Dockerfiles that require secrets (e.g., `GITHUB_TOKEN` for private dependencies), use `BuildSecret`:

```go
app, err := e2ekit.New(t).
    WithDockerfile(e2ekit.BuildConfig{
        ContextDir: "../../components/api",
        Dockerfile: "Dockerfile",
        Tag:        "my-api:test",  // Optional: custom tag
        Secrets: []e2ekit.BuildSecret{
            {ID: "github_token", Env: "GITHUB_TOKEN"},  // From environment variable
            // Or from file:
            // {ID: "npm_token", Src: "/path/to/secret/file"},
        },
    }).
    ExposePort(8080).
    WithWait(e2ekit.WaitHTTP(8080, "/health", 60*time.Second)).
    Run()
```

**Dockerfile usage:**

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.21

# Mount secret during build
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) && \
    git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/" && \
    go mod download
```

**Secret sources:**

| Field | Description |
|-------|-------------|
| `ID` | Secret identifier (used in Dockerfile `--mount=type=secret,id=<ID>`) |
| `Env` | Environment variable name (creates temp file automatically) |
| `Src` | Path to a file containing the secret |

**Note:** When secrets are specified, e2ekit uses the Docker CLI with BuildKit instead of testcontainers' built-in build (which doesn't support secrets).

### Wait Strategies

| Strategy | Description |
|----------|-------------|
| `WaitHTTP(port, path, timeout)` | Wait for HTTP 200 on endpoint |
| `WaitPort(port, timeout)` | Wait for port to be listening |
| `WaitLog(text, timeout)` | Wait for text in container logs |
| `WaitRunning(timeout)` | Minimal wait (container running) |

### Environment Rewriting

e2ekit automatically rewrites `localhost` to `host.docker.internal` in environment variables, solving the classic problem when tests run on the host but the app runs in a container.

```go
// This automatically transforms localhost URLs
app, _ := e2ekit.New(t).
    WithEnvVar("DATABASE_URL", "postgres://user:pass@localhost:5432/db").
    WithEnvRewriter(e2ekit.RewriteLocalhostToHostGateway()).
    Run()
// Inside container: postgres://user:pass@host.docker.internal:5432/db
```

## Chaos Metrics with metricskit

The `metricskit` addon provides observability and assertions for chaos engineering tests.

### Installation

```go
import "github.com/your-org/itestkit/addons/metricskit"
```

### Basic Usage

```go
func TestResilienceUnderChaos(t *testing.T) {
    ctx := context.Background()

    // Setup infrastructure with chaos
    pgInfra := postgres.NewPostgresInfra(postgres.PostgresConfig{
        Database:    "testdb",
        EnableProxy: true, // Required for chaos
    })

    suite, err := itestkit.New(t).
        WithChaos(itestkit.ChaosConfig{Enabled: true}).
        WithInfra(pgInfra).
        Build(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer suite.Terminate(ctx)

    // Create metrics collector
    metrics := metricskit.NewChaosMetrics()
    metrics.StartTest()

    // Run requests and record metrics
    for i := 0; i < 100; i++ {
        start := time.Now()
        err := doRequest(pgInfra.DSN())
        latency := time.Since(start)

        isTimeout := errors.Is(err, context.DeadlineExceeded)
        metrics.RecordRequest(err == nil, isTimeout, latency)
        if err != nil {
            metrics.RecordError(err.Error())
        }
    }

    // Inject chaos
    metrics.StartChaos()
    suite.Chaos().AddLatency(ctx, "pg-testdb-5432", 200*time.Millisecond, 50*time.Millisecond)

    // Run more requests during chaos
    for i := 0; i < 100; i++ {
        start := time.Now()
        err := doRequest(pgInfra.DSN())
        latency := time.Since(start)

        isTimeout := errors.Is(err, context.DeadlineExceeded)
        metrics.RecordRequest(err == nil, isTimeout, latency)
        if err != nil {
            metrics.RecordError(err.Error())
        }
    }

    metrics.EndChaos()
    metrics.EndTest()

    // Assert SLOs
    assertions := metricskit.Assert(metrics).
        SuccessRateAbove(90.0).
        P99Below(500 * time.Millisecond).
        TimeoutsBelow(10)

    if assertions.Failed() {
        t.Log(metricskit.Report(metrics).String())
        t.Fatal(assertions.Summary())
    }
}
```

### SLO Assertions

```go
// Create assertions from metrics
assertions := metricskit.Assert(metrics).
    SuccessRateAbove(95.0).           // At least 95% success rate
    P99Below(500 * time.Millisecond). // P99 latency under 500ms
    P95Below(200 * time.Millisecond). // P95 latency under 200ms
    AverageLatencyBelow(100 * time.Millisecond).
    ThroughputAbove(100.0).           // At least 100 req/s
    TimeoutsBelow(5).                 // Max 5 timeouts
    MinRequestsReached(1000)          // At least 1000 samples

// Check results
if assertions.Failed() {
    for _, result := range assertions.FailedResults() {
        t.Logf("[FAIL] %s: expected %s, got %s",
            result.Name, result.Expected, result.Actual)
    }
}
```

### Reporting

```go
// Full report to stdout
metricskit.Report(metrics).WriteReport(os.Stdout)

// Or get as string
report := metricskit.Report(metrics).String()
t.Log(report)

// Compact one-line summary for structured logs
summary := metricskit.Report(metrics).CompactSummary()
// Output: requests=200 success_rate=95.50% p99=450ms throughput=33.33/s chaos_duration=6s
```

### Error Classification

Errors are automatically classified into categories:

| Category | Patterns |
|----------|----------|
| `timeout` | timeout, deadline exceeded, i/o timeout |
| `connection` | connection refused, network unreachable |
| `refused` | connection refused, actively refused |
| `reset` | connection reset, reset by peer, broken pipe |
| `dns` | no such host, dns, lookup failed |
| `tls` | tls, certificate, x509, ssl |
| `server_error` | 500, 502, 503, 504, internal server error |
| `canceled` | context canceled, request canceled |
| `unknown` | Other errors |

```go
// Get error breakdown
counts := metrics.GetErrorCounts()
for category, count := range counts {
    t.Logf("%s: %d errors", category, count)
}
```

### Sample Report Output

```
╔══════════════════════════════════════════════════════════════╗
║                    CHAOS TEST REPORT                         ║
╠══════════════════════════════════════════════════════════════╣

  REQUEST METRICS
     Total Requests:      200
     Successful:          190
     Failed:              10
     Timeouts:            3
     Success Rate:        95.00%

  LATENCY METRICS
     Average:             125ms
     Min:                 12ms
     P50 (median):        98ms
     P90:                 245ms
     P95:                 312ms
     P99:                 456ms
     P99.9:               489ms

  THROUGHPUT
     Overall:             33.33 req/s
     Successful:          31.67 req/s
     During Chaos:        16.67 req/s

  DURATION
     Test Duration:       6s
     Chaos Duration:      3s

  ERROR BREAKDOWN
     timeout:             3
     connection:          5
     server_error:        2

╚══════════════════════════════════════════════════════════════╝
```

### E2E Performance Testing with Chaos

Combine `e2ekit` and `metricskit` for comprehensive API performance testing under chaos conditions:

```go
func TestAPIPerformanceUnderChaos(t *testing.T) {
    ctx := context.Background()

    // 1. Setup infrastructure with chaos enabled
    pgInfra := postgres.NewPostgresInfra(postgres.PostgresConfig{
        Database:    "apidb",
        Username:    "api",
        Password:    "secret",
        EnableProxy: true, // Required for chaos injection
    })

    suite, err := itestkit.New(t).
        WithChaos(itestkit.ChaosConfig{Enabled: true}).
        WithInfra(pgInfra).
        Build(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer suite.Terminate(ctx)

    // 2. Start the application container with e2ekit
    dsn, _ := pgInfra.DSN()
    app, err := e2ekit.New(t).
        WithImage("my-api:latest").
        ExposePort(8080).
        WithEnvVar("DATABASE_URL", dsn).
        WithEnvRewriter(e2ekit.RewriteLocalhostToHostGateway()).
        WithWait(e2ekit.WaitHTTP(8080, "/health", 30*time.Second)).
        Run()
    if err != nil {
        t.Fatal(err)
    }
    defer app.Terminate(ctx)

    // 3. Create metrics collector
    metrics := metricskit.NewChaosMetrics()
    client := &http.Client{Timeout: 5 * time.Second}

    // Helper to make API request and record metrics
    makeRequest := func() {
        start := time.Now()
        resp, err := client.Get(app.BaseURL + "/api/users")
        latency := time.Since(start)

        success := err == nil && resp != nil && resp.StatusCode == 200
        isTimeout := err != nil && strings.Contains(err.Error(), "timeout")

        metrics.RecordRequest(success, isTimeout, latency)
        if err != nil {
            metrics.RecordError(err.Error())
        }
        if resp != nil {
            resp.Body.Close()
        }
    }

    // 4. PHASE 1: Baseline performance (no chaos)
    t.Log("Phase 1: Measuring baseline performance...")
    metrics.StartTest()

    for i := 0; i < 50; i++ {
        makeRequest()
        time.Sleep(10 * time.Millisecond)
    }

    baselineSnapshot := metrics.Snapshot()
    t.Logf("Baseline: %s", metricskit.Report(baselineSnapshot).CompactSummary())

    // 5. PHASE 2: Performance under latency chaos
    t.Log("Phase 2: Injecting 200ms latency...")
    metrics.StartChaos()
    suite.Chaos().AddLatency(ctx, "pg-apidb-5432", 200*time.Millisecond, 50*time.Millisecond)

    for i := 0; i < 50; i++ {
        makeRequest()
        time.Sleep(10 * time.Millisecond)
    }

    // 6. PHASE 3: Performance under bandwidth chaos
    t.Log("Phase 3: Adding bandwidth limit (128 KB/s)...")
    suite.Chaos().RemoveAllToxics(ctx, "pg-apidb-5432")
    suite.Chaos().AddBandwidth(ctx, "pg-apidb-5432", 128)

    for i := 0; i < 50; i++ {
        makeRequest()
        time.Sleep(10 * time.Millisecond)
    }

    // 7. PHASE 4: Recovery (chaos removed)
    t.Log("Phase 4: Removing chaos, measuring recovery...")
    suite.Chaos().RemoveAllToxics(ctx, "pg-apidb-5432")
    metrics.EndChaos()

    for i := 0; i < 50; i++ {
        makeRequest()
        time.Sleep(10 * time.Millisecond)
    }

    metrics.EndTest()

    // 8. Generate final report
    t.Log("\n" + metricskit.Report(metrics).String())

    // 9. Assert SLOs for chaos resilience
    assertions := metricskit.Assert(metrics).
        SuccessRateAbove(85.0).            // Allow some degradation under chaos
        P99Below(2 * time.Second).         // P99 should still be reasonable
        P50Below(500 * time.Millisecond).  // Median shouldn't be too affected
        TimeoutsBelow(10).                 // Max 10 timeouts allowed
        MinRequestsReached(200)            // Ensure we have enough samples

    if assertions.Failed() {
        t.Log(assertions.Summary())
        t.Fatal("SLO assertions failed")
    }

    // 10. Compare baseline vs chaos performance
    chaosSnapshot := metrics.Snapshot()
    baselineP99 := baselineSnapshot.P99()
    chaosP99 := chaosSnapshot.P99()

    degradation := float64(chaosP99-baselineP99) / float64(baselineP99) * 100
    t.Logf("P99 degradation under chaos: %.2f%% (baseline=%v, chaos=%v)",
        degradation, baselineP99, chaosP99)

    // Alert if degradation is too high
    if degradation > 500 { // More than 5x slower
        t.Errorf("Excessive P99 degradation: %.2f%% (threshold: 500%%)", degradation)
    }
}
```

This test demonstrates:

| Phase | Description | Chaos Applied |
|-------|-------------|---------------|
| 1 | Baseline measurement | None |
| 2 | Latency injection | 200ms ± 50ms |
| 3 | Bandwidth limiting | 128 KB/s |
| 4 | Recovery measurement | None |

Key metrics captured:
- **Success rate** - How many requests succeeded under chaos
- **P99/P50 latency** - Tail vs median latency degradation
- **Throughput** - Requests per second during each phase
- **Error classification** - Types of failures (timeout, connection, etc.)

## Message Queue Testing with queuekit

The `queuekit` addon provides generic message queue consumption for E2E tests with support for generics, matchers, and multiple backends.

### Installation

```go
import "github.com/your-org/itestkit/addons/queuekit"
```

### Basic Usage

```go
// Define your message type
type JobNotification struct {
    JobID  string `json:"jobId"`
    Status string `json:"status"`
}

func TestWaitForJobCompletion(t *testing.T) {
    ctx := context.Background()

    // Setup RabbitMQ infrastructure
    rmqInfra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{
        Username: "guest",
        Password: "guest",
    })

    suite, err := itestkit.New(t).WithInfra(rmqInfra).Build(ctx)
    if err != nil {
        t.Fatal(err)
    }
    defer suite.Terminate(ctx)

    // Create AMQP consumer backend
    amqpURL, _ := rmqInfra.AMQPURL()
    backend, err := queuekit.NewAMQPConsumerBuilder(amqpURL).
        FromQueue("job.events").
        WithAutoAck(true).
        Build()
    if err != nil {
        t.Fatal(err)
    }

    // Create typed consumer with matcher
    consumer := queuekit.NewConsumer[JobNotification](t, backend).
        WithMatcher(queuekit.MatchJSONField("jobId", "job-123")).
        WithTimeout(30 * time.Second).
        Build()
    defer consumer.Close()

    // Wait for the specific job event
    msg, err := consumer.WaitForMessage(ctx)
    if err != nil {
        t.Fatal(err)
    }

    // Use the typed payload
    if msg.Payload.Status != "completed" {
        t.Errorf("expected completed, got %s", msg.Payload.Status)
    }
}
```

### Matchers

Matchers filter messages before unmarshaling, allowing efficient filtering based on headers, routing keys, or body content.

| Matcher | Description |
|---------|-------------|
| `MatchRoutingKey(key)` | Exact routing key match |
| `MatchRoutingKeyPrefix(prefix)` | Routing key starts with prefix |
| `MatchRoutingKeyPattern(regex)` | Routing key matches regex |
| `MatchHeader(key, value)` | Header has specific value |
| `MatchHeaderExists(key)` | Header key exists |
| `MatchCorrelationID(id)` | Correlation ID match |
| `MatchMessageID(id)` | Message ID match |
| `MatchBodyContains(substr)` | Body contains substring |
| `MatchBodyPattern(regex)` | Body matches regex |
| `MatchJSONField(path, value)` | JSON field equals value (supports dot notation) |
| `MatchJSONFieldExists(path)` | JSON field exists |
| `MatchJSONFieldPattern(path, regex)` | JSON field matches regex |
| `MatchContentType(type)` | Content type match |
| `MatchAll(matchers...)` | All matchers must match |
| `MatchAny(matchers...)` | At least one matcher must match |
| `MatchNone(matcher)` | Inverts matcher result |

```go
// Complex matching example
matcher := queuekit.MatchAll(
    queuekit.MatchRoutingKeyPrefix("job."),
    queuekit.MatchJSONField("status", "completed"),
    queuekit.MatchJSONFieldExists("result.path"),
)
```

### Wait Operations

```go
// Wait for a single message
msg, err := consumer.WaitForMessage(ctx)

// Wait for multiple messages
result, err := consumer.WaitForMessages(ctx, 5)
for _, m := range result.Messages {
    t.Logf("Job %s: %s", m.Payload.JobID, m.Payload.Status)
}

// Capture all messages for a duration
result, err := consumer.CaptureAll(ctx, 10*time.Second)
t.Logf("Captured %d messages", result.Count())

// Assert no messages arrive (useful for negative tests)
err := consumer.AssertNoMessages(ctx, 5*time.Second)

// Drain queue before test
count, err := consumer.DrainQueue(ctx, 5*time.Second)
t.Logf("Drained %d old messages", count)
```

### Convenience Functions

```go
// One-liner for simple waits
msg, err := queuekit.WaitFor[JobNotification](
    ctx,
    t,
    backend,
    queuekit.MatchJSONField("jobId", jobID),
    30*time.Second,
)

// Wait for N messages
result, err := queuekit.WaitForN[JobNotification](
    ctx,
    t,
    backend,
    queuekit.MatchRoutingKey("job.completed"),
    5,
    30*time.Second,
)
```

### Assertions

```go
// Assert on single message
queuekit.AssertMessage(t, msg).
    HasRoutingKey("job.completed").
    HasCorrelationID("corr-123").
    PayloadSatisfies("has valid path", func(p JobNotification) bool {
        return p.Result != nil && p.Result.Path != ""
    })

// Assert on result
queuekit.AssertResult(t, result).
    HasCount(5).
    HasNoErrors().
    DidNotTimeout().
    First().HasRoutingKey("job.completed")

// Fluent expectations
queuekit.ExpectMessages(t, result).
    ToSucceed().
    ToHaveCount(5).
    ToContainWhere("has job-123", func(p JobNotification) bool {
        return p.JobID == "job-123"
    }).
    OrFatal()
```

### AMQP Consumer Builder

```go
backend, err := queuekit.NewAMQPConsumerBuilder(amqpURL).
    FromQueue("my-queue").
    BindTo("my-exchange", "routing.key.#").  // Optional binding
    WithAutoAck(true).
    WithExclusive(false).
    WithPrefetch(10).
    WithQueueDeclare(true, false).  // durable=true, autoDelete=false
    Build()
```

### Publishing Messages (Test Setup)

```go
publisher, err := queuekit.NewAMQPPublisher(amqpURL)
if err != nil {
    t.Fatal(err)
}
defer publisher.Close()

// Publish test message
body, _ := json.Marshal(map[string]any{"jobId": "test-123"})
err = publisher.Publish(ctx, "my-exchange", body,
    queuekit.WithRoutingKey("job.created"),
    queuekit.WithCorrelationID("corr-123"),
    queuekit.WithPersistent(),
)
```

### Custom Backend Implementation

Implement `QueueConsumer` interface for other message brokers:

```go
type QueueConsumer interface {
    Consume(ctx context.Context) (<-chan Message, error)
    Close() error
}

// Example: Kafka, Redis Streams, SQS, etc.
type MyKafkaConsumer struct { /* ... */ }

func (c *MyKafkaConsumer) Consume(ctx context.Context) (<-chan queuekit.Message, error) {
    // Implementation
}

func (c *MyKafkaConsumer) Close() error {
    // Implementation
}
```

### E2E Example: Job Processing Pipeline

```go
func TestJobProcessingE2E(t *testing.T) {
    ctx := context.Background()

    // 1. Setup infrastructure
    rmqInfra := rabbitmq.NewRabbitInfra(rabbitmq.RabbitConfig{})
    pgInfra := postgres.NewPostgresInfra(postgres.PostgresConfig{})

    suite, _ := itestkit.New(t).
        WithInfra(rmqInfra).
        WithInfra(pgInfra).
        Build(ctx)
    defer suite.Terminate(ctx)

    // 2. Start application
    app, _ := e2ekit.New(t).
        WithImage("job-processor:latest").
        WithEnvVar("AMQP_URL", rmqInfra.AMQPURL()).
        WithEnvVar("DATABASE_URL", pgInfra.DSN()).
        Run()
    defer app.Terminate(ctx)

    // 3. Setup queue consumer for events
    backend, _ := queuekit.NewAMQPConsumerBuilder(rmqInfra.AMQPURL()).
        FromQueue("job.events").
        Build()

    consumer := queuekit.NewConsumer[JobNotification](t, backend).
        WithTimeout(60 * time.Second).
        WithDebugLog(true).
        Build()
    defer consumer.Close()

    // 4. Trigger job via API
    jobID := triggerJob(t, app.BaseURL)

    // 5. Wait for completion event
    msg, err := consumer.WaitForMessage(ctx)
    if err != nil {
        t.Fatalf("job didn't complete: %v", err)
    }

    // 6. Assert result
    queuekit.AssertMessage(t, msg).
        PayloadSatisfies("completed successfully", func(p JobNotification) bool {
            return p.Status == "completed" && p.JobID == jobID
        })
}
```

## Automatic Host Normalization

All infrastructure `HostPort()` methods automatically normalize `localhost` and `127.0.0.1` to the Docker gateway IP, so containers can reach services on the host.

```go
// No need to manually handle localhost replacement
host, port, _ := pgInfra.HostPort()
// Returns "172.17.0.1:5432" on Linux, or resolves host.docker.internal on Docker Desktop
```

This is handled automatically by `itestkit.NormalizeHost()` which:

1. Checks `TESTCONTAINERS_HOST_OVERRIDE` environment variable (if set)
2. Uses `host.docker.internal` on Docker Desktop
3. Uses Docker bridge gateway IP on Linux (typically `172.17.0.1`)
4. Falls back to DNS resolution of `host.docker.internal`

## CI/CD Considerations

### Docker-in-Docker (DinD)

When running tests in CI/CD with Docker-in-Docker, the automatic host resolution usually works correctly because the Docker daemon runs inside the CI container.

### Docker Socket Mount

When mounting the Docker socket (`/var/run/docker.sock`) from the host, you may need to set `TESTCONTAINERS_HOST_OVERRIDE` to ensure containers can reach each other:

```yaml
# GitHub Actions
env:
  TESTCONTAINERS_HOST_OVERRIDE: "host.docker.internal"

# GitLab CI
variables:
  TESTCONTAINERS_HOST_OVERRIDE: $CI_JOB_CONTAINER_IP

# Kubernetes
env:
  - name: TESTCONTAINERS_HOST_OVERRIDE
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
```

### Testcontainers Cloud / Remote Docker

For remote Docker environments, set `TESTCONTAINERS_HOST_OVERRIDE` to the appropriate address that containers should use to reach the test runner.

## Best Practices

1. **Use pre-built infra** for convenience, **generic containers** for control
2. **Always enable chaos** in CI to catch resilience issues early
3. **Use wait strategies** (`WaitHTTP`, `WaitLog`) instead of `time.Sleep`
4. **Separate test types**: integration (infra + code) vs E2E (infra + app container)
5. **Test failure scenarios**: latency spikes, connection drops, timeouts
6. **Let itestkit handle host resolution** — don't manually replace localhost in tests

## What itestkit Is NOT

- A mocking framework
- A replacement for unit tests
- A heavy DSL for infrastructure

**itestkit is an orchestrator for real test environments.**