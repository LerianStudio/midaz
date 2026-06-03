# Contributing to itestkit

This guide explains how to evolve the framework, infra, and addons for itestkit. It follows the architecture described in `pkg/itestkit/README.md` and prioritizes real infrastructure, isolation, reproducibility, and chaos.

## Architecture principles

- Core agnostic: the root package does not know specific technologies; details live in `infra/` and `addons/`.
- Builder without side effects: only `Build()` starts containers, proxies, and resources.
- Reproducibility: use deterministic wait strategies; avoid `time.Sleep` when possible.
- Optional chaos: proxies are created only when `ChaosConfig.Enabled` and infra requests it.
- Predictable shutdown: `Terminate()` must be idempotent and release resources in reverse order.

## Package map

```
pkg/itestkit/
  README.md
  infra.go              // Infra, NamedInfra
  suite.go              // Builder, Suite, Env
  chaos.go              // ChaosConfig, ChaosInterface
  chaos_toxiproxy.go    // toxiproxy implementation
  container_generic.go  // ContainerSpec, WithContainerCustomize
  customizer.go
  customizer_options.go
  infra/
    postgres/
    mysql/
    mongodb/
    redis/
    rabbitmq/
    mssql/
    oracle/
  addons/
    e2ekit/
    metricskit/
    queuekit/
```

## Evolving the core (pkg/itestkit)

- `Infra` and `NamedInfra` in `infra.go`: implement `InfraKind()` and `InfraName()` to avoid duplicates; default `Name` should be "default".
- `Builder`/`Suite`/`Env` in `suite.go`: `Build()` creates the environment and `Terminate()` shuts down in reverse order; keep `Builder` pure and side-effect free.
- `Env` and the contract with tests: use `Env.Containers` for generic containers; additions to `Env` must be backward compatible and documented.
- `ContainerSpec` in `container_generic.go`: new capabilities should prefer `Customizer` or `WaitStrategy` before adding fields.
- `Customizer` in `customizer*.go`: only adjust `testcontainers.ContainerRequest`, no I/O; use `uniqueAppendMany` to avoid duplicates.

## Evolving chaos

- `ChaosInterface` in `chaos.go` defines the contract; `chaos_toxiproxy.go` is the default implementation.
- New providers: create `chaos_<provider>.go`, implement the contract, and add selection via `ChaosConfig` without breaking the default.
- Proxy names must be stable. For generic containers use `<prefix>-<container>-<port>`. For infra with one port, use `<prefix>-<name>` (or include the port if it is part of the standard) and document it in the README.

## Evolving infra (pkg/itestkit/infra/<name>)

1. Create the package in `pkg/itestkit/infra/<name>` with `<name>.go`, `<name>_options.go`, and tests in `<name>_test.go`.
2. Define `<Name>Config` with `Name`, `Image`, credentials, `EnableProxy`, `ProxyName`, and `Options`.
3. In `New<Name>Infra`, apply defaults (image, credentials, `Name=default`, `ProxyName` with a clear prefix).
4. In `Start`, initialize the container (prefer `testcontainers-go/modules` when available), get `Host` and `MappedPort`, build the `Upstream`, and create a proxy if `EnableProxy` and `env.Chaos != nil`.
5. Expose `Endpoint()` and helpers (`DSN()`, `URL()`, or `Addr()`) using the final address (proxy when present).
6. Implement `Terminate`, `InfraKind`, and `InfraName` to enable duplicate validation in the `Builder`.
7. In `<name>_options.go`, use the pattern `type <Name>Option func(*<name>Options)` with `runOpts []testcontainers.ContainerCustomizer` to allow customization.
8. Write basic tests with `context.WithTimeout` and real connectivity validation; use `t.Skip` when the environment requires it.
9. Update `pkg/itestkit/README.md` with the new infra (architecture and examples).

## Evolving addons (pkg/itestkit/addons/<addon>)

1. Create the package in `pkg/itestkit/addons/<addon>` and keep dependency on the core only when needed.
2. Prefer a fluent API (builder) with safe defaults; avoid side effects before `Run()`/`Start()`.
3. For container-based addons, offer `WaitStrategy` and failure logs (like `e2ekit`).
4. For metrics/observability addons, provide small, thread-safe interfaces (like `metricskit`).
5. Add tests and examples and update `pkg/itestkit/README.md`.

## Tests and validation

- Recommended: `go test ./pkg/itestkit/...`
- Infra tests depend on Docker; use timeouts and deterministic waits.

## PR checklist

- Public API backward compatible or migration documented.
- README updated with new infra/addons/usage.
- Tests added and executed.
- Proxy names and `InfraName` are stable.
