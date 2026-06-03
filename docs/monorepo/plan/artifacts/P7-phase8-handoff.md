# P7 -> P8 readiness handoff

The coordination artifact P7-T17 requires. States Phase 7's exit state for Phase 8
so P8 does not re-derive the preconditions. **P7 GATES P8**: P8 may not begin until
the full P7 exit-criteria set (below) is green.

- Phase: P7 (unified go.mod / monorepo convergence)
- Branch: `phase/p7-gomod-unification`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Exit state of P7 (the gate)

The unified module is converged:

- **One module, one graph.** `github.com/LerianStudio/midaz/v3`, `go 1.26.3`, no
  `toolchain` directive. NO `replace` directives, NO `go.work` file — single-version
  per shared path (`go list -m all` is single-version for every shared module path;
  `validator/v9` is the only allowlisted second-major case, documented in
  `P7-second-major-allowlist.md`).
- **Build / vet green.** `go build ./...` and `go vet ./...` exit 0 over the full
  unified surface (P7-T08).
- **Unit green.** Full-tree `make test-unit` green (P7-T10).
- **Integration green (runnable subset proven this chunk; see "Integration"
  below).** The third-rail fee re-proof (P7-T18) is 23/23 green against real
  Postgres+Mongo+Redis+RabbitMQ; tracer audit hash-chain and CRM holder/alias
  integration green; reporter-worker RabbitMQ integration green. Reporter PDF e2e
  is DEFERRED to P8 (needs the P8-owned SeaweedFS service).
- **Static third-rail guard green.** The T27 AST structural gate
  (`transaction_fee_seam_structure_test.go`) passes: single `validate`
  reassignment + seam-precedes-redis-seed, with bite tests proving each gate fails
  on a contrived break (P7-T18a).
- **mongo-driver v1+v2 coexist** in one graph (`go.mongodb.org/mongo-driver
  v1.17.9` + `.../v2 v2.6.0`) with no codec/BSON runtime issue surfaced by the
  CRM (v1) and reporter-worker (v2) integration runs (R14 verified at execution,
  not assumed).

## `github_token` / private-module machinery is REMOVABLE in Phase 8

P7-T16 proved the unified module resolves with ZERO private Lerian modules, in a
clean environment (no `~/.netrc`, no `GOPRIVATE`/`GONOSUMDB`/`GOINSECURE` for
`github.com/LerianStudio/*`, default public proxy):

- `env -u GOPRIVATE -u GONOSUMDB GOPROXY=https://proxy.golang.org,direct go mod
  download` -> **exit 0**.
- `go mod verify` -> **"all modules verified"** (exit 0).
- `go env GOPRIVATE GOFLAGS GONOSUMDB` -> all empty (no private-module env carried).
- Every `github.com/LerianStudio/*` module in the graph is public:
  `midaz/v3` (the module itself), `lib-auth/v2 v2.8.0`, `lib-commons/v5 v5.4.1`,
  `lib-observability v1.0.1`, `lib-streaming v1.4.0` — all resolve via the public
  proxy. The only formerly-private import was fees' (P4) `midaz/v3`, which is now
  the module itself.
- The 5 component Dockerfiles in the merged tree carry NO `--mount=type=secret`,
  NO `GITHUB_TOKEN`, NO `GOPRIVATE`, NO `.netrc` dance — they already build against
  the public proxy.

### Phase 8 MAY DELETE (precondition met, evidence above)

- the `github_token` BuildKit secret,
- the `.secrets/` directory dance,
- the `~/.netrc` setup step,
- the `go_private_modules` workflow inputs,

from the incoming Dockerfiles/workflows. No code path needs authenticated module
fetch. This is the P7-A finding, now re-confirmed on the converged tree.

## What P8 inherits (scope P8 owns)

1. **4-image fan-out finalization.** `build.yml` already fans out 5 candidates
   (`crm`, `ledger`, `tracer`, `reporter-manager`, `reporter-worker`) via
   `filter_paths`, but the deploy-key maps are still ledger+crm only. P8 finalizes
   the deploy topology: the merged binary set is ledger (unified: +crm +fees)
   plus tracer, reporter-manager, reporter-worker.
2. **SeaweedFS / KEDA service definitions.** The reporter PDF e2e (the T17-equivalent
   reporter proof) needs SeaweedFS; the reporter-worker autoscaling needs KEDA.
   Both service defs are P8-owned. The reporter PDF e2e is DEFERRED to P8 for this
   reason (see "Integration" below).
3. **Helm / gitops key maps for the relocated components.** The `midaz` Helm chart
   (`helm_values_key_mappings`) and `midaz-firmino-gitops` (`yaml_key_mappings`)
   must ADD `tracer` / `reporter-manager` / `reporter-worker` and REMOVE the
   `crm` / `plugin-fees` image keys (crm and fees are folded into ledger). Today
   the maps are `{"midaz-crm": "crm", "midaz-ledger": "ledger"}` (build.yml) and the
   gitops equivalent — both still carry the pre-collapse `crm` key. This is the
   P7-T19 / P8 Helm-gitops-APIDog lockstep (R12).
4. **godog CI job (P7-T15).** Per `P7-godog-ci.md`: a BESPOKE, non-gating,
   tracer-path-filtered midaz-local workflow running `make test-bdd`. The decision
   is final; P8 implements the job and must not re-litigate shared-vs-bespoke.
5. **Coverage 85%-gate go-live.** Deferred from P5/P6; P8 flips the coverage gate
   to gating across the unified surface.
6. **`make build` covering all deploy units.** P8 ensures the build surface covers
   every deploy unit (4 binaries) so a co-located component cannot build-green yet
   ship nowhere.

## Integration: ran vs deferred (this chunk)

| Suite | Infra | Result |
|---|---|---|
| Ledger fee re-proof (P7-T18, 23 classes) | testcontainers pg+mongo+redis+rabbit | GREEN 23/23 |
| Tracer audit hash-chain (`VerifyHashChain`, genesis/chain/dedup/post-migration) | testcontainer pg + in-process tracer service | GREEN 6/6 |
| CRM holder + alias mongodb | testcontainer mongo | GREEN 49/49 |
| Reporter-worker RabbitMQ retry/DLQ consumer (mongo-driver v2 component) | testcontainer rabbit | GREEN 4/4 |
| Reporter PDF e2e (T17-equivalent) | **SeaweedFS (P8-owned)** | **DEFERRED to P8** |

All testcontainer runs require `ALLOW_INSECURE_TLS=true` (mongo/redis adapters
enforce TLS otherwise) — a harness env requirement, not a code property.

## Verification commands (reference)

```
env -u GOPRIVATE -u GONOSUMDB GOPROXY=https://proxy.golang.org,direct go mod download   # exit 0
go mod verify                                                                            # all modules verified
go build ./... && go vet ./...                                                           # exit 0
ALLOW_INSECURE_TLS=true go test -tags integration -run 'TestFeeProof|TestFeeHarness_Sanity' \
  ./components/ledger/internal/adapters/http/in/ -v                                      # 23/23 green
go test ./components/ledger/internal/adapters/http/in/ -run FeeSeamStructure             # 4/4 green
```
