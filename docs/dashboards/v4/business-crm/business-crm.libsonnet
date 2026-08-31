// Midaz Ledger — Business & CRM dashboard.
//
// Domain metrics, bulk recorder integrity, data access and CRM field protection.
// Every query was validated against live series in Mimir; see ../telemetry-dictionary.md.

local d = import '../../lib/dashboard.libsonnet';

local app = d.selector.app;
local pos = d.pos;

// Non-production write volume is sparse and bursty. rate() per second would round every
// one of these panels to zero, so business counters use increase().
//
// The window is $__rate_interval, never $__interval. increase() needs two samples inside
// its window; at the 1h and 6h ranges these panels are read at, $__interval resolves to
// well under a minute — shorter than the export interval — and every panel returns empty.
// Measured against a live environment: [30s] and [1m] yield no series, [2m] yields 10. See
// ../telemetry-dictionary.md § Emission continuity.
local windowed(metric) = 'sum(increase(%s{%s}[$__rate_interval]))' % [metric, app];
local windowedBy(label, metric) = 'sum by (%s) (increase(%s{%s}[$__rate_interval]))' % [label, metric, app];
local rangeTotal(metric) = 'sum(increase(%s{%s}[$__range]))' % [metric, app];
local ratedBy(label, metric) = 'sum by (%s) (rate(%s{%s}[$__rate_interval]))' % [label, metric, app];

// Business counts are derived from domain_operations_total rather than from
// transactions_processed_total / accounts_created_total. Those two series still exist in
// Mimir but appear nowhere in the Go source — they are legacy series inside retention,
// and a panel built on them would read zero permanently.
local rangeOp(op) = 'sum(increase(domain_operations_total{%s, operation="%s", result="success"}[$__range]))' % [app, op];
local windowedOp(op) = 'sum(increase(domain_operations_total{%s, operation="%s", result="success"}[$__rate_interval]))' % [app, op];

d.dashboard(
  'midaz-v4-business',
  'Midaz · Ledger · Business & CRM (v4)',
  |||
    Domain metrics, bulk recorder, data access and CRM field protection for the midaz v4
    unified ledger binary, which serves ledger, CRM and fees under a single job. Pick the
    target environment with the Namespace variable.

    In low-traffic environments a blank panel usually means "no activity in this window"
    rather than "no telemetry". Counters reset whenever a pod is replaced, and writes tend to
    arrive in bursts separated by health-probe-only idle. Business panels therefore use
    increase() over $__rate_interval rather than per-second rate(), which would round to
    zero, or $__interval, which is shorter than the export interval and returns nothing at
    all. See the version telemetry dictionary, § Emission continuity.

    Source of truth: this dashboard is generated from
    docs/dashboards/v4/business-crm/business-crm.libsonnet — edit there, not in the UI.
  |||,
  ['midaz', 'ledger', 'crm', 'business', 'v4'],
  [
    d.row('Transactions & Ledger', 0),

    d.stat(
      'Transactions created',
      rangeOp('create_transaction'),
      pos(0, 1, 6, 4),
      {
        decimals: 0,
        description: 'Successful create_transaction domain operations over the selected period. Derived from domain_operations_total, not from transactions_processed_total: the latter is a legacy series still inside Mimir retention that the current binary no longer emits.',
      }
    ),

    d.stat(
      'Accounts created',
      rangeOp('create_account'),
      pos(6, 1, 6, 4),
      {
        decimals: 0,
        description: 'Successful create_account domain operations over the selected period. Derived from domain_operations_total for the same reason as the panel to its left.',
      }
    ),

    d.stat(
      'Balances synced',
      rangeTotal('balance_synced_total'),
      pos(12, 1, 6, 4),
      { decimals: 0, description: 'Balance synchronisations completed by the worker.' }
    ),

    d.stat(
      'Domain operations',
      rangeTotal('domain_operations_total'),
      pos(18, 1, 6, 4),
      { decimals: 0, description: 'Total domain operations executed over the selected period.' }
    ),

    d.timeSeries(
      'Business volume',
      [
        d.promTarget(windowedOp('create_transaction'), 'transactions created', 'A'),
        d.promTarget(windowedOp('create_account'), 'accounts created', 'B'),
        d.promTarget(windowed('balance_synced_total'), 'balances synced', 'C'),
      ],
      pos(0, 5, 24, 8),
      { description: 'Uses increase() per interval rather than rate(): non-production write volume is low enough that a per-second rate would round everything to zero.' }
    ),

    d.row('Domain Operations', 13),

    d.timeSeries(
      'Operations by type',
      [d.promTarget(windowedBy('operation', 'domain_operations_total'), '{{operation}}')],
      pos(0, 14, 12, 9),
      {
        stack: true,
        legend: 'table',
        description: 'By operation: create_transaction, create_account, calculate_fee, list_ledgers and similar.',
      }
    ),

    d.timeSeries(
      'Operations by result',
      [d.promTarget(windowedBy('result', 'domain_operations_total'), '{{result}}')],
      pos(12, 14, 12, 9),
      {
        stack: true,
        description: 'Success versus failure at the domain layer. This is the only business error signal available: span status_code is always STATUS_CODE_UNSET, so a span-derived error rate does not work.',
      }
    ),

    d.timeSeries(
      'p95 duration by operation',
      [d.promTarget(
        'histogram_quantile(0.95, sum by (le, operation) (rate(domain_operation_duration_ms_milliseconds_bucket{%s}[$__rate_interval])))' % app,
        '{{operation}}'
      )],
      pos(0, 23, 12, 9),
      {
        unit: 'ms',
        fill: 0,
        legend: 'table',
        description: 'p95 duration of each domain operation, isolating business-rule cost from I/O cost.',
      }
    ),

    d.timeSeries(
      'Operations by component',
      [d.promTarget(windowedBy('component', 'domain_operations_total'), '{{component}}')],
      pos(12, 23, 12, 9),
      {
        stack: true,
        description: 'Distribution across components of the unified binary: onboarding, transaction, crm, fees.',
      }
    ),

    d.row('Bulk Recorder', 32),

    d.timeSeries(
      'Transactions: attempted vs inserted',
      [
        d.promTarget(windowed('bulk_recorder_transactions_attempted_total'), 'attempted', 'A'),
        d.promTarget(windowed('bulk_recorder_transactions_inserted_total'), 'inserted', 'B'),
      ],
      pos(0, 33, 8, 8),
      { fill: 0, description: 'The two lines must overlap. Any separation between them is lost batch writes.' }
    ),

    d.timeSeries(
      'Operations: attempted vs inserted',
      [
        d.promTarget(windowed('bulk_recorder_operations_attempted_total'), 'attempted', 'A'),
        d.promTarget(windowed('bulk_recorder_operations_inserted_total'), 'inserted', 'B'),
      ],
      pos(8, 33, 8, 8),
      { fill: 0, description: 'Same contract at the accounting-operation level.' }
    ),

    d.stat(
      'Insertion gap (transactions)',
      'round(%s - %s)' % [rangeTotal('bulk_recorder_transactions_attempted_total'), rangeTotal('bulk_recorder_transactions_inserted_total')],
      pos(16, 33, 8, 4),
      {
        decimals: 0,
        description: 'Attempted minus inserted, rounded. A positive value means a transaction was accepted but never reached the database. Both operands are increase() over the range, which extrapolates to the window boundaries independently, so the raw difference carries sub-unit noise even when nothing was lost; round() drops it and keeps the displayed value and the red threshold in agreement. Only increase() is safe here — the counters reset on every pod restart.',
        steps: [
          { color: 'green', value: null },
          { color: 'red', value: 1 },
        ],
      }
    ),

    d.stat(
      'Batch p95',
      'histogram_quantile(0.95, sum by (le) (rate(bulk_recorder_bulk_duration_ms_milliseconds_bucket{%s}[$__rate_interval])))' % app,
      pos(16, 37, 8, 4),
      { unit: 'ms', decimals: 1, description: 'p95 time to write one batch.' }
    ),

    d.row('Balance Sync', 41),

    d.stat(
      'Sync staleness',
      'time() - max(balance_sync_last_success_timestamp_seconds{%s})' % app,
      pos(0, 42, 6, 4),
      {
        unit: 's',
        decimals: 0,
        description: 'Age of the last batch sync that ran to completion, from the heartbeat gauge the worker stamps per scope. A failure counter cannot see a silent stall — nothing fails because nothing runs — which is exactly what this panel exists to show. "No data" is expected until the first completed batch since pod boot, and on builds that predate the gauge. Thresholds are starting points, not SLOs: pair with backlog before treating staleness as an incident — an idle environment with nothing scheduled is legitimately stale.',
        steps: [
          { color: 'green', value: null },
          { color: 'orange', value: 900 },
          { color: 'red', value: 3600 },
        ],
      }
    ),

    d.stat(
      'Deltas lost (expired)',
      'round(sum(increase(balance_sync_orphan_dropped_total{%s, reason="expired"}[$__range])))' % app,
      pos(6, 42, 6, 4),
      {
        decimals: 0,
        description: 'Scheduled sync keys whose Redis value expired before the flush. Each one is a pending balance delta that was never persisted and is unrecoverable — this is data loss, not noise. reason="unparseable" (a key-format regression) is charted on the panel below but excluded here so this stat stays a pure loss signal.',
        steps: [
          { color: 'green', value: null },
          { color: 'red', value: 1 },
        ],
      }
    ),

    d.stat(
      'Batch failures',
      'round(%s)' % rangeTotal('balance_sync_batch_failures_total'),
      pos(12, 42, 6, 4),
      {
        decimals: 0,
        description: 'Batch sync failures over the selected period. Each failure is a batch of balances not persisted to PostgreSQL: reads served from PostgreSQL fall behind until a later batch succeeds. The counter only materialises after its first increment, so blank means "never failed since boot", not "not deployed".',
        steps: [
          { color: 'green', value: null },
          { color: 'red', value: 1 },
        ],
      }
    ),

    d.stat(
      'Tenant skips',
      'round(%s)' % rangeTotal('balance_sync_tenant_skip_total'),
      pos(18, 42, 6, 4),
      {
        decimals: 0,
        description: 'Tenants the multi-tenant worker skipped because their database connection could not be resolved. A tenant counted here is not syncing at all. Always zero (or absent) in single-tenant deployments.',
        steps: [
          { color: 'green', value: null },
          { color: 'red', value: 1 },
        ],
      }
    ),

    d.timeSeries(
      'Synced vs dropped',
      [
        d.promTarget(windowed('balance_synced_total'), 'synced', 'A'),
        d.promTarget(windowedBy('reason', 'balance_sync_orphan_dropped_total'), 'dropped ({{reason}})', 'B'),
      ],
      pos(0, 46, 12, 8),
      {
        fill: 0,
        description: 'Balances persisted versus scheduled keys dropped without persisting. dropped(expired) is lost data; dropped(unparseable) is a key-format regression. Both should sit at zero while the synced line follows write volume.',
      }
    ),

    d.timeSeries(
      'Failure counters',
      [
        d.promTarget(windowed('balance_sync_batch_failures_total'), 'batch failures', 'A'),
        d.promTarget(windowed('balance_sync_cleanup_failures_total'), 'cleanup failures', 'B'),
        d.promTarget(windowed('balance_sync_tenant_skip_total'), 'tenant skips', 'C'),
      ],
      pos(12, 46, 12, 8),
      {
        fill: 0,
        description: 'Cleanup failures mean keys the flush had finished with were not removed from the schedule, so the next cycle reprocesses them — wasteful, not incorrect, and NOT proof the balances were persisted: the all-orphans path also cleans up without writing. Read alongside "Synced vs dropped" to tell the two apart.',
      }
    ),

    d.row('Data Access', 54),

    d.timeSeries(
      'Reads by source',
      [d.promTarget(windowedBy('source', 'db_read_source_total'), '{{source}}')],
      pos(0, 55, 12, 8),
      {
        stack: true,
        description: 'Read origin, primary versus replica. A drop in replica reads pushes load onto the primary.',
      }
    ),

    d.timeSeries(
      'Redis backup queue depth',
      [d.promTarget('redis_backup_queue_depth_ratio{%s}' % app, '{{k8s_pod_name}}')],
      pos(12, 55, 12, 8),
      {
        fill: 4,
        description: 'Despite the _ratio suffix inherited from WithUnit("1"), this value is an absolute count of queued items. Sustained growth means a stalled consumer.',
      }
    ),

    d.row('CRM Protection / KMS', 63),

    d.timeSeries(
      'Protection status',
      [d.promTarget(ratedBy('status', 'crm_protection_status_total'), '{{status}}')],
      pos(0, 64, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Field-protection status per CRM record evaluated by the unified ledger binary. status=none means records carry no protected fields; a shift off none means field protection started applying.',
      }
    ),

    d.timeSeries(
      'Mode resolution',
      [d.promTarget(ratedBy('mode', 'crm_protection_mode_resolution_total'), '{{mode}}')],
      pos(12, 64, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Encryption mode resolved by the unified ledger binary: legacy (lib-commons symmetric crypto) versus envelope (Vault Transit with Tink), selected by KMS_VENDOR.',
      }
    ),

    d.timeSeries(
      'Encrypt / decrypt operations',
      [d.promTarget(ratedBy('outcome', 'crm_protection_encrypt_decrypt_total'), '{{outcome}}')],
      pos(0, 72, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Field-crypto volume by result inside the unified ledger binary. Any outcome other than success is PII that failed to be read or written.',
      }
    ),

    d.timeSeries(
      'Legacy-format reads',
      [d.promTarget('sum(rate(crm_protection_legacy_read_total{%s}[$__rate_interval]))' % app, 'legacy reads')],
      pos(12, 72, 12, 8),
      {
        unit: 'reqps',
        description: 'Reads still served from the legacy format by the unified ledger binary. Under migration to envelope mode this line should trend to zero.',
      }
    ),
  ]
)
