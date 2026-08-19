# Active-account billing over streaming

> **Status: operations runbook.** This document is the single source of truth for how the ledger
> emits the `active_account` billable event, how you enable it per environment, how you verify a pod
> can bill, and what its durability limits are. Everything below is a **fact grounded in code** in
> *this* repo, cited inline.
>
> **Scope.** Billing emission runs in-process inside the ledger binary on `:3002`. It piggybacks on
> the same lib-streaming producer that carries every other ledger event; there is no separate service,
> image, or port. A Schema Registry and a Kafka/Redpanda broker are **external** dependencies you
> supply per environment.
>
> **Citation convention.** Unprefixed anchors name a symbol you can grep. Line ranges rot, so the
> durable anchors are the cited **function, const, and file** names — grep those, not line numbers.

---

## 1. Overview — what fires, and when

The ledger emits a Lago-metering billable event named `active_account`. It fires **once per unique
internal account** touched by a **posted (APPROVED)** transaction.

The derivation lives in `buildActiveAccountBillingPayloads`
(`components/ledger/internal/services/command/billing_active_account.go`):

- A transaction that is `nil` or whose status is not `APPROVED` yields **nothing**.
- Each operation contributes its account, **de-duplicated** by account ID in first-seen order.
- External-account legs are **excluded** — any operation whose alias starts with the external-account
  prefix (`constant.DefaultExternalAccountAliasPrefix`, i.e. `@external/*`) is skipped.

Emission is driven by `SendActiveAccountBillingEvents`, which early-returns on the no-op lifecycle
phase (`TransactionLifecyclePhaseNoop`) so a redelivery with no eligible status transition never
double-emits. This keeps billing at idempotency parity with `SendTransactionEvents`.

### Payload and wire format

Each payload is a `billing.BillablePayload` with these fields:

| Field | Value |
|---|---|
| `Metric` | `active_account` (const `activeAccountMetric`) |
| `SubscriptionId` | the billing subscription: the tenant ID when multi-tenant is enabled, otherwise the transaction's organization ID |
| `Properties.account_id` | the internal account ID |
| `Properties.transaction_id` | the transaction ID |

The payload is **not JSON**. The billing serializer encodes it as **Confluent-framed Protobuf** — a
leading `0x00` magic byte plus the schema ID, then the Protobuf body. Billing does **not** use the
`ToEmitRequest` / `EmitImportant` JSON path that the domain events use; it emits raw Protobuf bytes
directly through the `Emitter` seam.

> **Approved exception to the "IMPORTANT direct-emits go through `EmitImportant`" convention.**
> Billing intentionally calls `Emitter.Emit` directly with a raw Confluent-Protobuf `Payload` rather
> than routing through `pkgStreaming.EmitImportant` / `ToEmitRequest` (the JSON CloudEvents path),
> because billable events are raw Protobuf and cannot use the JSON payload builder. The IMPORTANT-posture
> mechanics that `EmitImportant` normally provides — a bounded per-emit context, span-error recording,
> warn-and-swallow on failure, and the nil-emitter/nil-serializer guard — are provided **inline at the
> emit site** in `SendActiveAccountBillingEvents` / `emitActiveAccountBillingEvent`
> (`billing_active_account.go`). This is a reviewed, deliberate exception, not an oversight.

### Topic and CloudEvents envelope

All active-account events land on a single shared topic:

```
lerian.streaming.billing.recorded
```

This is a fixed literal topic owned by lib-streaming's `billing` package, wired explicitly via
`RouteOverrides(billing.Route())` in `BuildStreamingEmitter`
(`components/ledger/internal/bootstrap/streaming.go`) — **not** the ledger's application topic
`lerian.streaming.ledger`, which every other ledger event rides and which is derived from the
service's `ce-source`. The billing event is the one destination the ledger writes whose name does
not come from the application name.

| CloudEvents attribute | Value |
|---|---|
| `ce-type` | `studio.lerian.ledger.billing.recorded` — the `ledger` segment is the producing application, stamped from `ce-source` |
| `ce-source` | `ledger` |
| `ce-subject` | the account ID |
| `ce-datacontenttype` | `application/vnd.confluent.protobuf` |
| `ce-tenantid` | resolved from the request context |

---

## 2. Enabling billing in an environment

Billing emits **only** when both of these hold:

1. `STREAMING_ENABLED=true` — the master streaming flag. When false, a `NoopEmitter` is injected and
   no broker connection is attempted, so billing is off.
2. A Schema Registry is **configured and reachable at bootstrap**. Without one, the billing serializer
   is nil and billing is off (see §3).

### The three Schema Registry env vars

These are read by **lib-streaming's `LoadConfig`**, not by midaz directly. They are declared in
`components/ledger/.env.example` under `--- Schema Registry (billing emits) ---`:

| Variable | Purpose | Default |
|---|---|---|
| `STREAMING_SCHEMA_REGISTRY_URL` | Schema Registry endpoint. Empty ⇒ billing disabled. | empty |
| `STREAMING_SCHEMA_REGISTRY_USERNAME` | Optional basic-auth username. | empty |
| `STREAMING_SCHEMA_REGISTRY_PASSWORD` | Optional basic-auth password. | empty |

**Safe by default.** All three ship empty. An empty URL disables billing and **never crashes the
app** — the service boots and serves normally with billing off.

**Both-or-neither credentials.** `USERNAME` and `PASSWORD` are a pair. A partial pair (one set, one
empty) is rejected fail-closed by `NewSchemaRegistryClient`, which disables billing rather than
connecting half-authenticated. The password is **never logged**.

**Use HTTPS when credentials are set.** When `USERNAME`/`PASSWORD` are supplied, point
`STREAMING_SCHEMA_REGISTRY_URL` at an `https://` endpoint. Basic-auth credentials sent over plaintext
`http://` are exposed on the wire; lib-streaming surfaces a warning for cleartext credentials. Plain
`http://` is acceptable only for an unauthenticated local registry (e.g. compose Redpanda) with no
credentials set.

### Local vs deployed

- **Local (compose):** point the URL at Redpanda's built-in Schema Registry, which listens on `:8081`:
  ```
  STREAMING_SCHEMA_REGISTRY_URL=http://midaz-redpanda:8081
  ```
  In-network containers reach it at `http://midaz-redpanda:8081`.
- **Deployed:** the URL, username, and password are supplied per environment through the external
  deploy repos (see §6). Absent ⇒ billing off (safe).

---

## 3. Verifying a pod can bill

Billing readiness is decided **once at bootstrap** and surfaced in the boot log.
`buildBillingSerializer` (`streaming.go`) resolves — and on first run self-registers — the schema
subject `lerian.streaming.billing.recorded-value`, then injects the serializer.

Two distinct outcomes leave billing off, and only one of them logs:

- **Streaming disabled** (`STREAMING_ENABLED=false`, i.e. `cfg.Enabled` is false):
  `buildBillingSerializer` returns a **nil serializer silently** — no `warnBillingDisabled` call, so
  **no WARN is emitted**. Billing is simply not wired because streaming itself is off.
- **Streaming on but the registry failed** (context canceled, empty URL or partial credentials,
  registry round-trip failure): the branch degrades to a **nil serializer** and emits exactly one WARN
  through `warnBillingDisabled`:

```
WARN  Billing serializer disabled  billing_enabled=false  error=<cause>
```

The `billing_enabled=false` WARN therefore specifically means **streaming is on but the Schema
Registry could not be wired** — it is not the signal for "streaming disabled".

**The check.** Grep the boot log for `billing_enabled`:

- The `Billing serializer disabled` WARN with `billing_enabled=false` is present ⇒ **streaming is on
  but the registry failed**, so billing is off. Read the attached `error` and re-check the registry
  URL and its reachability.
- No `Billing serializer disabled` line means either billing booted **on** cleanly, **or** streaming
  is disabled entirely (no WARN in that case). Confirm `STREAMING_ENABLED` to tell the two apart.

**Asymmetric dependency.** The Schema Registry is needed **at boot only**. Once the serializer
resolves the schema, `Serialize` and `Emit` do **no** registry I/O on the request path — a registry
outage after boot does not affect in-flight transactions, and a registry outage at boot only disables
billing (it never blocks startup).

---

## 4. Durability and limitations

Billing is **best-effort, fire-and-forget**. It is wired to **never** fail the parent transaction.

- **`RouteRequired` is not delivery durability.** `billing.Route()` is `RouteRequired`, which enforces
  route-table completeness at **Build** time — the Builder refuses to construct if the route is
  missing. It says nothing about whether any individual event is delivered.
- **No outbox.** No outbox is wired for billing (a ratified deferral). There is no transactional
  guarantee that an emitted event reaches the broker.
- **Drop on failure.** A broker outage, an absent `lerian.streaming.billing.recorded` topic, or a
  serialize/emit failure is span-recorded, warn-logged, and **dropped**. `SendActiveAccountBillingEvents`
  and `emitActiveAccountBillingEvent` swallow the error and return; the transaction still succeeds.
- **A lost billing event is a lost revenue signal** until an outbox lands. Adding one is a future
  decision.

Operationally: a healthy transaction path can still silently under-bill if the broker or topic is
unavailable. Monitor the `billing serialize failed` and `billing emit failed` WARN lines and the
recorded spans to catch drops.

---

## 5. Per-environment provisioning

Two things must exist per environment before billing works:

1. **A reachable Schema Registry** — its URL configured via `STREAMING_SCHEMA_REGISTRY_URL` (§2). The
   serializer self-registers the `lerian.streaming.billing.recorded-value` subject on first run, so the
   registry must be reachable at ledger boot.
2. **The `lerian.streaming.billing.recorded` topic pre-created.** lib-streaming has **no** client-side
   auto-create — an absent topic means dropped events, not an auto-provisioned one.

### Local

The `midaz-redpanda-init` compose service (`components/infra/docker-compose.yml`) pre-creates the
billing topic and its DLQ alongside the two application topics (`lerian.streaming.ledger` and
`lerian.streaming.tracer`, each with its own `.dlq`):

```
rpk topic create lerian.streaming.billing.recorded -r 1 -p 1 --brokers midaz-redpanda:9092
rpk topic create lerian.streaming.billing.recorded.dlq -r 1 -p 1 --brokers midaz-redpanda:9092
```

Redpanda's built-in Schema Registry serves the local registry on `:8081`.

### Deployed

Topic creation is the platform's topic-provisioning responsibility for the target environment. Create
`lerian.streaming.billing.recorded` through the same mechanism you use for the application topics, and
point `STREAMING_SCHEMA_REGISTRY_URL` at the environment's Schema Registry.

---

## 6. Deployment and external coordination

### Deploy repos (external)

This repo has **no** in-tree Helm chart or Terraform. The ledger deployment surfaces the three Schema
Registry keys per environment from the external **LerianStudio deploy repos**:

| Key | How it is surfaced |
|---|---|
| `STREAMING_SCHEMA_REGISTRY_URL` | values / ConfigMap (plain value per environment) |
| `STREAMING_SCHEMA_REGISTRY_USERNAME` | Kubernetes Secret — **never** a plaintext value |
| `STREAMING_SCHEMA_REGISTRY_PASSWORD` | Kubernetes Secret — **never** a plaintext value |

If these keys are absent, billing is off and the service still runs (safe default). Add them per
environment to turn billing on.

### Lago prerequisite (external — owner: finance / billing-api)

Before the emitted events produce revenue, a metric must exist in the billing tenant:

- A Lago billable metric with code **`active_account`** must exist in the billing tenant.
- Its aggregation type is chosen by finance — for example `unique_count` over the `account_id`
  property, or `count`.
- **Subscription keying.** Lago's subscription must be keyed by the **tenant ID** (multi-tenant) or
  the **organization ID** (single-tenant), matching the `SubscriptionId` this ledger emits. The
  `active_account` metric increments **once per event**; the individual account is available in
  `Properties.account_id` (and in `ce-subject`) for `unique_count`-style aggregations.

`billing-api` decodes the Confluent-Protobuf payload **best-effort** and forwards it to Lago with **no
re-validation**, dead-lettering on Lago rejection. A missing or misconfigured `active_account` metric
in Lago therefore surfaces as DLQ entries in billing-api, not as ledger errors.

---

## Related documents

- [CRM Field Encryption & KMS Key Management](crm-field-encryption.md) — the sibling deep doc whose
  structure this runbook mirrors.
- `components/ledger/.env.example` — the canonical env-var reference for the `STREAMING_*` knobs.
