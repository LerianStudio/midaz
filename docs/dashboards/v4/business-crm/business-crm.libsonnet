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

    d.row('Data Access', 41),

    d.timeSeries(
      'Reads by source',
      [d.promTarget(windowedBy('source', 'db_read_source_total'), '{{source}}')],
      pos(0, 42, 12, 8),
      {
        stack: true,
        description: 'Read origin, primary versus replica. A drop in replica reads pushes load onto the primary.',
      }
    ),

    d.timeSeries(
      'Redis backup queue depth',
      [d.promTarget('redis_backup_queue_depth_ratio{%s}' % app, '{{k8s_pod_name}}')],
      pos(12, 42, 12, 8),
      {
        fill: 4,
        description: 'Despite the _ratio suffix inherited from WithUnit("1"), this value is an absolute count of queued items. Sustained growth means a stalled consumer.',
      }
    ),

    d.row('CRM Protection / KMS', 50),

    d.timeSeries(
      'Protection status',
      [d.promTarget(ratedBy('status', 'crm_protection_status_total'), '{{status}}')],
      pos(0, 51, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Field-protection status per CRM record evaluated by the unified ledger binary. status=none means records carry no protected fields; a shift off none means field protection started applying.',
      }
    ),

    d.timeSeries(
      'Mode resolution',
      [d.promTarget(ratedBy('mode', 'crm_protection_mode_resolution_total'), '{{mode}}')],
      pos(12, 51, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Encryption mode resolved by the unified ledger binary: legacy (lib-commons symmetric crypto) versus envelope (Vault Transit with Tink), selected by KMS_VENDOR.',
      }
    ),

    d.timeSeries(
      'Encrypt / decrypt operations',
      [d.promTarget(ratedBy('outcome', 'crm_protection_encrypt_decrypt_total'), '{{outcome}}')],
      pos(0, 59, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        description: 'Field-crypto volume by result inside the unified ledger binary. Any outcome other than success is PII that failed to be read or written.',
      }
    ),

    d.timeSeries(
      'Legacy-format reads',
      [d.promTarget('sum(rate(crm_protection_legacy_read_total{%s}[$__rate_interval]))' % app, 'legacy reads')],
      pos(12, 59, 12, 8),
      {
        unit: 'reqps',
        description: 'Reads still served from the legacy format by the unified ledger binary. Under migration to envelope mode this line should trend to zero.',
      }
    ),
  ]
)
