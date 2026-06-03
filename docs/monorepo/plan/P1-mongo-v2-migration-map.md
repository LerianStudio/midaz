# mongo-driver v1.17.9 → v2.6.0 Migration Map (midaz)

**Phase:** P1 (re-characterized) — lib-commons `v5.4.1` pin + mongo-driver v1→v2 migration, atomic, runs first.
**Status of this doc:** the verified migration contract Gate-0 implementation executes from. Every API-delta claim is grounded against the cached `go.mongodb.org/mongo-driver/v2@v2.6.0` source, not memory.
**Decision (Fred, 2026-06-03):** pin v5.4.1, migration-first (land fully current). `lib-observability` HELD at `v1.0.1` (PD-4). Branch `chore/p1-libcommons-ga-v5.4.1`.

**Why the migration is forced:** lib-commons v5.4.1's `commons/mongo/mongo.go` and `commons/tenant-manager/core/context.go` moved to `mongo-driver/v2`. `tmcore.GetMBContext(ctx) → *v2/mongo.Database` and `libMongo.Client.Database(ctx) → (*v2/mongo.Database, error)` now hand midaz v2 types. midaz's mongo layer is v1 throughout. The two driver majors are distinct module paths, so their types are mutually incompatible — every midaz file in the chain must move to v2 in lockstep. The break is a **type-identity change** (`*mongo.Database` text identical, package identity flipped) — invisible to static symbol/signature audits, caught only by the compiler.

---

## 1. Per-file inventory (~60 files)

**Convention:** every import `go.mongodb.org/mongo-driver/X` → `go.mongodb.org/mongo-driver/v2/X`. `bson/primitive` is **deleted in v2**; its symbols moved into `bson`.

### Group A — Foundation: pure bson helpers (no driver-connection, no lib-commons join)
| File | v1 symbols | Notes |
|------|-----------|-------|
| `pkg/mongo/mongo.go` | `bson.M` (BuildDocumentToPatch, flattenBSONM) | Pure map manipulation. Import swap only. |
| `pkg/mongo/mongo_test.go` | `bson.M` | Import swap. |
| `pkg/net/http/httputils.go` | `bson.M` (`*bson.M` field on `QueryHeader`) | Import swap. The `http.QueryHeader` type consumed by every metadata adapter signature. |
| `pkg/net/http/httputils_test.go` | `bson.M` | Import swap. |

### Group B — bson-only consumers (handlers + service queries)
HTTP handlers (all `bson.M` only): `http/in/{account,accounttype,asset,assetrate,balance,ledger,operation,operation_route,organization,portfolio,segment,transaction,transaction_route,transaction_query_handlers}.go` + `observability_test.go`.
Service query layer: `services/query/{get_all_account_type,get_all_operation_routes,get_all_transaction_routes}.go` (`bson.M`) + tests `get_all_metadata_*_test.go`, `get_all_operation_routes_test.go`, `get_all_operations_test.go`, `get_all_transaction_routes*_test.go`, `get_external_id_assetrate_test.go`, `get_id_operation_route_test.go` (`bson` + `bson/primitive`).
Model files (the `primitive.ObjectID` carriers):
- `components/ledger/internal/adapters/mongodb/transaction/metadata.go` + `metadata_test.go`
- `components/ledger/internal/adapters/mongodb/onboarding/metadata.go` + `metadata_test.go`
  - Fields: `ID primitive.ObjectID bson:"_id,omitempty"` → `bson.ObjectID`; `Key primitive.D` → `bson.D`.

### Group C — Connection/bootstrap (lib-commons join + index models)
| File | v1 symbols | Migration |
|------|-----------|-----------|
| `components/ledger/internal/bootstrap/config.mongo.transaction.go` | `bson`, `mongo` (`IndexModel`), `options.Index()` | Import swap + `IndexModel.Options` type (§2.5). |
| `components/ledger/internal/bootstrap/config.mongo.onboarding.go` | same | same |
| `components/ledger/internal/bootstrap/readyz_checkers.go` | `bson.D` (ping cmd) | Import swap only. |

### Group D — Ledger metadata adapters (heavy joins: getDatabase + full CRUD)
| File | v1 symbols | Weight |
|------|-----------|--------|
| `.../mongodb/transaction/metadata.mongodb.go` | `bson.{M,D,E}`, `mongo.{Database,Collection,IndexModel,ErrNoDocuments,Pipeline,WriteModel,NewUpdateOneModel,CommandError}`, `options.{Update,BulkWrite,Find,Delete,Index,ListIndexes}` | Heaviest. getDatabase return type, insertMetadataChunk param, all CRUD + options builders. |
| `.../mongodb/onboarding/metadata.mongodb.go` | same | Heaviest (twin). |
| `metadata.mongodb_integration_test.go` (×2), `metadata.mongodb_tenant_test.go` (×2), `metadata_bulk_benchmark_test.go` (×2) | `bson`, `mongo`, `options` | Migrate with adapter. |

### Group E — CRM mongo adapters (alias + holder)
| File | v1 symbols | Migration |
|------|-----------|-----------|
| `crm/.../alias/alias.mongodb.go` | `bson`, `mongo.{Database,IsDuplicateKeyError}`, `options.Delete` | getDatabase + CRUD. |
| `crm/.../alias/alias_query.mongodb.go` | `bson`, `options.Find().SetLimit/SetSkip/SetSort` | options builder. |
| `crm/.../alias/alias_maintenance.mongodb.go` | `bson`, `mongo.{Collection,IndexModel}`, `options.Index()` ×11 | createIndexes param + IndexModel.Options. |
| `crm/.../holder/holder.mongodb.go` | mirror of alias | getDatabase + CRUD. |
| `crm/.../holder/holder_query.mongodb.go` | `bson`, `options.Find()...` | options builder. |
| `crm/.../holder/holder_maintenance.mongodb.go` | `bson`, `mongo.{Collection,IndexModel}`, `options.Index()` | createIndexes + IndexModel.Options. |
| alias/holder `_integration_test.go`, `_tenant_test.go`, `_test.go` | `bson`, `mongo`, `options` | Migrate with adapters. |

### Group F — Service command (error classification)
| File | v1 symbols | Migration |
|------|-----------|-----------|
| `services/command/create_metadata_bulk.go` | `mongo.{IsTimeout,IsNetworkError}` | Import swap only (helpers unchanged). |

### Group G — Shared test utilities (testcontainer fixtures — migrate or all integration tests fail to compile)
| File | v1 symbols | Migration |
|------|-----------|-----------|
| `tests/utils/mongodb/container.go` | `mongo.{Client,Database,Connect}`, `options.Client().ApplyURI` | `mongo.Connect` ctx-drop (§2.4). |
| `tests/utils/mongodb/fixtures.go` | `bson.M`, `primitive.{ObjectID,NewObjectID}`, `mongo.Database` | primitive→bson; CreateConnection/InsertMetadata signatures. |

---

## 2. Exact v1→v2 API delta catalog (verified against v2.6.0)

### 2.1 Import-path moves
All `mongo-driver/{bson,mongo,mongo/options}` → `/v2/` prefix. `mongo-driver/bson/primitive` has **no v2 equivalent — package deleted**.

### 2.2 `primitive` → `bson` merge (highest volume: 44 sites)
`bson/primitive` dir absent in v2; symbols in `bson/objectid.go` + `bson/primitive.go`.

| v1 | v2 | count |
|----|----|-------|
| `primitive.ObjectID` | `bson.ObjectID` (`[12]byte`) | 8 |
| `primitive.NewObjectID()` | `bson.NewObjectID()` | 33 |
| `primitive.ObjectIDFromHex(s)` | `bson.ObjectIDFromHex(s)` | 1 |
| `primitive.D` | `bson.D` (`[]bson.E`) | 2 |
| `primitive.NilObjectID` | `bson.NilObjectID` | as needed |

`bson.E` unchanged: `{Key string; Value any}`. `bson.M = map[string]any`, `bson.D = []E`, `bson.A = []any` — identical shape. All `bson.M`/`bson.D`/`bson.E` sites are pure import-path swaps.

### 2.3 options builder rework
`options.Client()` is **unchanged** (returns `*ClientOptions`; `ApplyURI`/`SetMaxPoolSize`/etc. survive). Find/Update/Delete/Index/etc. constructors now return `*XOptionsBuilder`; collection methods take `...options.Lister[options.XOptions]` (the builder satisfies `Lister`, so call sites passing `opts` still compile). The one true rename:

| v1 | v2 | sites | Action |
|----|----|-------|--------|
| `options.Update()` | **`options.UpdateOne()`** | 4 (`metadata.mongodb.go` ×2 per component) | **RENAME** — all feed `UpdateOne`: `options.UpdateOne().SetUpsert(true)`. `options.Update()` does not exist in v2. |
| `options.Find()` | `*FindOptionsBuilder` | many | No edit (name preserved; SetLimit/SetSkip/SetSort present). |
| `options.Delete()` | `*DeleteOptionsBuilder` | 2 | Verify DeleteOne vs DeleteMany pairing at call site. |
| `options.BulkWrite().SetOrdered(false)` | same | 2 | No edit. |
| `options.Index()` | `*IndexOptionsBuilder` | 19 | No edit at constructor; `IndexModel.Options` field type changed (§2.5). |
| `options.ListIndexes()` | `*ListIndexesOptionsBuilder` | 2 | No edit. |

SetX confirmed present in v2: `SetUpsert`, `SetOrdered`, `SetLimit/SetSkip/SetSort`, `SetUnique/SetName/SetExpireAfterSeconds/SetSparse`.

### 2.4 `mongo.Connect` — ctx-drop
v1 `Connect(ctx, opts...)` → v2 `Connect(opts...)` (no ctx), then `client.Ping(ctx, nil)`.
- **midaz production code: ZERO direct `mongo.Connect` sites** — all connection logic is inside lib-commons `commons/mongo` (already v2). The only midaz site is `tests/utils/mongodb/container.go` → drop the `ctx` arg.

### 2.5 cursor / result / index / write-model
| Symbol | v2 status |
|--------|-----------|
| `cursor.All/Decode/Next`, `SingleResult.Decode` | unchanged signatures |
| `mongo.ErrNoDocuments` | unchanged |
| `mongo.IndexModel{Keys, Options}` | `Options` type: `*options.IndexOptions` → `*options.IndexOptionsBuilder`; since `options.Index()` returns the builder, literals compile as-is |
| `mongo.NewUpdateOneModel().SetFilter().SetUpdate().SetUpsert()` | all present |
| `mongo.WriteModel`, `[]mongo.WriteModel`, `mongo.Pipeline` | unchanged |
| `mongo.{InsertOneResult,UpdateResult,DeleteResult,BulkWriteResult}` | present; verify field names at decode sites |
| `mongo.IsDuplicateKeyError/IsTimeout/IsNetworkError` | unchanged |
| `mongo.CommandError` | struct, value receiver `Error()` (both versions) → `errors.As(err, &cmdErr)` with `var cmdErr mongo.CommandError` works unchanged; `.Name`/`.Code` present |

---

## 3. Implementation/dependency order

The lib-commons join is the spine. Two midaz functions consume v2 types from lib-commons:
- `getDatabase(ctx) (*mongo.Database, error)` — assigns from `tmcore.GetMBContext` (v2) and `connection.Database` (v2). **Hard join.**
- `insertMetadataChunk(ctx, coll *mongo.Collection, ...)` and CRM `createIndexes(ctx, collection *mongo.Collection)` — receive `*mongo.Database.Collection()` output (v2). **Hard join.**

**No midaz code passes a mongo type INTO lib-commons** — flow is strictly lib-commons → midaz. One-directional ⇒ midaz only *receives* v2 types, never *constructs* one to hand back.

**Safe sequence (each slice builds green before the next):**
1. Add `go.mongodb.org/mongo-driver/v2 v2.6.0` as a direct require (without bumping lib-commons yet) — v2 imports resolve while beta.12 still compiles the rest. Both drivers coexist until the last file migrates.
2. **Foundation (Group A):** `pkg/mongo`, `pkg/net/http/httputils.go`. Build `./pkg/...`.
3. **Test utils (Group G):** `tests/utils/mongodb`. Build `-tags integration ./tests/...`.
4. **Bootstrap + bson-only consumers (B, C, F):** handlers, service queries, models, index config, error helpers.
5. **Adapters (D, E) + flip lib-commons v5.4.1 + tidy** — ATOMIC (the join only closes when both sides speak v2). Drop residual v1 driver. Full `go build ./...`.

> **Orchestrator decision:** original Slices 4 and 5 are FOLDED into one atomic build-step (adapters can't build green without the lib-commons flip). 4 implementation slices total.

---

## 4. Semantic / behavioral gotchas (compile-clean, runtime-different)

1. **ObjectID JSON wire shape — SAFE (verified).** v1 and v2 both emit `"<24hex>"`. Metadata `_id` in API responses does not drift.
2. **bson `omitempty` on zero `ObjectID` in upsert.** `ID bson.ObjectID bson:"_id,omitempty"` — a zero ObjectID must be OMITTED so Mongo server-assigns `_id`. **At-risk:** `transaction/metadata.mongodb.go`, `onboarding/metadata.mongodb.go` Create + BulkWrite. Integration test must assert a non-zero `_id` round-trips.
3. **`ObjectID.IsZero()` / `NilObjectID`.** Any branch on a zero ObjectID must use `bson.NilObjectID`/`.IsZero()`. **At-risk:** `metadata.go` model construction.
4. **Decode strictness.** v2's default decoder is stricter on type mismatch. **At-risk:** metadata adapter `FindList`/`FindByEntity` decode sites — integration tests must decode a real document, not mocks.
5. **`Connect` is lazy in v2** (no server round-trip; needs explicit `Ping`). lib-commons owns production Connect. **At-risk only:** `tests/utils/mongodb/container.go` — after ctx-drop, ensure the readiness `Ping` still runs.
6. **`options.Merge*` removed in v2.** midaz uses none. Flagged so impl doesn't reach for them.

---

## 5. Integration-test surface (Gate-9 acceptance bar)

**HAVE integration + tenant tests (`//go:build integration`, via `make test-integration`):**
- crm alias: `alias.mongodb_integration_test.go` + `alias.mongodb_tenant_test.go`
- crm holder: `holder.mongodb_integration_test.go` + `holder.mongodb_tenant_test.go`
- ledger transaction: `metadata.mongodb_integration_test.go` + `metadata.mongodb_tenant_test.go` (+ bulk benchmark)
- ledger onboarding: `metadata.mongodb_integration_test.go` + `metadata.mongodb_tenant_test.go`

**Gaps / appropriate-as-is:**
- `pkg/mongo/mongo.go` — pure helpers, unit-tested (correct per CLAUDE.md). Add ONE integration assertion that a patch document survives a real upsert (covers gotcha #2/#4).
- `tests/utils/mongodb/` — infra, proven transitively when adapter suites pass.
- `services/query/get_all_*` — mock-unit-tested; real decode/query lives in adapters (covered).

**Gate-9 bar:** `make test-integration` green across all four mongo adapter suites (regular + tenant) on v5.4.1, **plus** two new integration assertions: (a) zero-ObjectID upsert yields server-assigned `_id`; (b) full document decode round-trips through the v2 decoder.

---

## 6. Gate-0 slices (4, each ending green; v1+v2 coexist until Slice 4)

- **Slice 1 — Foundation bson + driver require.** Add `mongo-driver/v2 v2.6.0`. Migrate `pkg/mongo/mongo.go`, `pkg/net/http/httputils.go` (+tests). Build `./pkg/...`; test `./pkg/mongo/... ./pkg/net/http/...`.
- **Slice 2 — Test utilities.** `tests/utils/mongodb/{container.go,fixtures.go}` (primitive→bson, Connect ctx-drop). Build `-tags integration ./tests/...`.
- **Slice 3 — bson-only consumers + bootstrap.** Groups B, C, F: handlers, service-query files+tests, both `metadata.go` models, `config.mongo.*.go`, `readyz_checkers.go`, `create_metadata_bulk.go`. Build touched `./components/...`.
- **Slice 4 (folds original 4+5) — Adapters + lib-commons flip.** Groups D, E + `go get lib-commons/v5@v5.4.1 && go mod tidy` + drop v1 driver. The `options.Update()→options.UpdateOne()` rename (4 sites), getDatabase return types, Collection params. Full gate: `go build ./...`, `go vet ./...`, `make lint`, `make test-unit`, `make test-integration` (mongo suites), `make sec`, `go test ./pkg/streaming/...`, `go mod verify`, `GOFLAGS=-mod=readonly go build ./...`.
