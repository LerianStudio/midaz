# Grafana dashboards

Dashboards for midaz, authored as Jsonnet and compiled to Grafana JSON.

```
docs/dashboards/
├── lib/dashboard.libsonnet          # shared panel/dashboard helpers, version-independent
├── v4/
│   ├── telemetry-dictionary.md      # the v4 metric contract these dashboards bind to
│   ├── golden-signals/              # RED + USE for the ledger API
│   └── business-crm/                # domain metrics, bulk recorder, CRM field protection
└── v3/                              # not yet authored — see its README
```

## Why dashboards are scoped by midaz version

The telemetry contract is not stable across major versions, so one dashboard cannot serve
both. In **v4** the ledger binary is unified: ledger, CRM and fees all emit under
`job="ledger"`. In **v3** they are separate deployments — `ledger`, a standalone
`midaz-crm`, and `plugin-fees` — so the same panel needs a different `job` selector, and
some metrics do not exist at all on one side or the other. Each version therefore carries
its own dashboards *and* its own `telemetry-dictionary.md`, and the verifier checks each
dashboard against the dictionary for its own version.

Environments are a variable, not a directory: every dashboard exposes Namespace and
Datasource dropdowns, so one build serves dev, staging and production alike.

The `.libsonnet` files are the source of truth. The compiled `<theme>.json` is a build
artifact and is gitignored — do not commit it, and do not edit dashboards in the Grafana
UI expecting the change to survive.

## Build

```bash
go install github.com/google/go-jsonnet/cmd/jsonnet@v0.22.0   # once
make dashboards          # compile every theme to JSON
make dashboards-verify   # check every referenced metric is documented
```

## Why not grafonnet

These dashboards do not import `github.com/grafana/grafonnet`. That library needs
jsonnet-bundler plus a vendored tree of several thousand files, which is disproportionate
for two dashboards and would put a large third-party vendor directory in this repo. The
helpers in `lib/dashboard.libsonnet` emit plain Grafana schema v39 JSON, so CI needs one
static binary and nothing else.

## Publishing

Publishing is manual today — either import the JSON through the Grafana UI, or POST it to
the target folder:

```bash
curl -sS -X POST "$GRAFANA_URL/api/dashboards/db" \
  -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --slurpfile d docs/dashboards/v4/golden-signals/golden-signals.json \
        --arg folder "$GRAFANA_FOLDER_UID" \
        '{dashboard: $d[0], folderUid: $folder, overwrite: true}')"
```

On first import Grafana prompts for the Datasource and Logs variables; they are dashboard
variables, so nothing is pinned to a particular Grafana instance.

There is no automated deploy. Adding one means giving CI a Grafana service-account token,
which is a decision for whoever owns the cluster, not something to smuggle in with a
dashboard change.

## What CI checks, and what it deliberately does not

`.github/workflows/dashboards.yml` runs two mechanical checks on any PR touching this
directory:

1. **Compile** — every theme must produce valid JSON.
2. **Primitive verification** — every metric referenced by a panel must appear in the
   `telemetry-dictionary.md` belonging to that dashboard's version.

Check 2 exists because of a concrete failure found while building these: the dashboards
originally charted `transactions_processed_total` and `accounts_created_total`, which are
still present in Mimir but appear nowhere in the Go source. They are legacy series inside
retention, and those panels would have read zero forever without ever looking broken.

**Not installed: a dictionary-versus-code drift gate.** The
`ring:creating-grafana-dashboards` skill specifies one, but its regenerate script depends
on a `ring telemetry-inventory` CLI that does not exist. Committing it would have added a
blocking check that fails on every PR. When that CLI ships, the drift gate is the natural
next step; until then `telemetry-dictionary.md` is maintained by hand and its
`_meta.generation` field says so.

## Adding a panel

1. Confirm the metric exists on the wire, not just in code. A metric that compiles is not
   a metric that is emitted:
   ```promql
   count(count_over_time({__name__="your_metric_total", k8s_namespace_name="<namespace>"}[1d]))
   ```
   Use `count_over_time` over a day rather than a bare `count`. Series are intermittent in
   low-traffic environments, and an instant query tests whether traffic is flowing right
   now, not whether the metric exists.
2. Add it to that version's `telemetry-dictionary.md` under the right section, with its
   declaration `file:line` and its observed labels.
3. Add the panel to the theme `.libsonnet`.
4. `make dashboards && make dashboards-verify`.

Read the "Wire-name translation" and "Legacy series" sections of the dictionary first.
The declared name in Go is frequently not the series name in Mimir.
