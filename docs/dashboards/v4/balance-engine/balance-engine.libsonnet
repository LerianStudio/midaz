// Midaz Ledger — Balance engine dashboard.
//
// The accounting adapter is currently an internal, inactive port. These panels are
// intentionally useful when the port is enabled in a local or instrumented environment;
// an empty panel today means that no engine invocation has emitted a sample.

local d = import '../../lib/dashboard.libsonnet';
local app = d.selector.app;
local pos = d.pos;

local requests = 'balance_engine_requests_total';
local failures = 'balance_engine_failures_total';
local recovery = 'balance_engine_recovery_total';
local duration = 'balance_engine_duration_ms_milliseconds';
local recoveryDuration = 'balance_engine_recovery_duration_ms_milliseconds';
local requestBytes = 'balance_engine_request_size_bytes';
local poolSize = 'balance_engine_pool_balance_count';
local touchedSize = 'balance_engine_touched_balance_count';

local rate(metric, extra='') =
  if extra == '' then
    'sum(rate(%s{%s}[$__rate_interval]))' % [metric, app]
  else
    'sum by (%s) (rate(%s{%s}[$__rate_interval]))' % [extra, metric, app];

local quantile(metric, q, extra='') =
  local grouping = if extra == '' then 'le' else 'le, ' + extra;
  'histogram_quantile(%s, sum by (%s) (rate(%s_bucket{%s}[$__rate_interval])))' % [q, grouping, metric, app];

d.dashboard(
  'midaz-v4-balance-engine',
  'Midaz · Balance Engine (v4)',
  |||
    Internal accounting-adapter RED/USE signals: invocation outcomes, bounded failures,
    CAS/preflight attempts, request and recovery latency, payload size, and pool/touched
    cardinality. The execution port is currently unset, so the absence of samples is
    expected until a local opt-in or instrumented environment invokes the adapter.
    Queries use only bounded labels (outcome, code, type) and $__rate_interval.

    Source of truth: docs/dashboards/v4/balance-engine/balance-engine.libsonnet.
  |||,
  ['midaz', 'ledger', 'balance-engine', 'v4'],
  [
    d.row('Invocation outcomes and failure modes', 0),

    d.timeSeries(
      'Requests by outcome',
      [d.promTarget(rate(requests, 'outcome'), '{{outcome}}')],
      pos(0, 1, 12, 8),
      { unit: 'reqps', legend: 'table', description: 'Accounting adapter invocations by bounded outcome: success, refused, technical_error, or indeterminate.' }
    ),

    d.timeSeries(
      'Failures by protocol code',
      [d.promTarget(rate(failures, 'code'), '{{code}}')],
      pos(12, 1, 12, 8),
      { unit: 'reqps', legend: 'table', description: 'Recognized closed-vocabulary failure classifications. No IDs, aliases, or monetary values are labels.' }
    ),

    d.timeSeries(
      'Recovery outcomes',
      [d.promTarget(rate(recovery, 'outcome'), '{{outcome}}')],
      pos(0, 9, 12, 8),
      { unit: 'reqps', legend: 'table', description: 'Recovery finalization outcomes, including finalization and acknowledgment failures.' }
    ),

    d.timeSeries(
      'Indeterminate outcomes',
      [d.promTarget(rate('balance_engine_indeterminate_total'))],
      pos(12, 9, 12, 8),
      { unit: 'reqps', description: 'Invocations whose accounting result could not be confirmed.' }
    ),

    d.row('Duration and request shape', 17),

    d.timeSeries(
      'Adapter duration p95 / p99',
      [
        d.promTarget(quantile(duration, '0.95'), 'p95', 'A'),
        d.promTarget(quantile(duration, '0.99'), 'p99', 'B'),
      ],
      pos(0, 18, 12, 9),
      { unit: 'ms', fill: 0, description: 'Complete adapter invocation duration, including validation and normalization.' }
    ),

    d.timeSeries(
      'Recovery duration p95 / p99',
      [
        d.promTarget(quantile(recoveryDuration, '0.95', 'outcome'), 'p95 {{outcome}}', 'A'),
        d.promTarget(quantile(recoveryDuration, '0.99', 'outcome'), 'p99 {{outcome}}', 'B'),
      ],
      pos(12, 18, 12, 9),
      { unit: 'ms', fill: 0, description: 'Recovery finalization duration by bounded outcome.' }
    ),

    d.timeSeries(
      'Validated request bytes p95 / p99',
      [
        d.promTarget(quantile(requestBytes, '0.95'), 'p95', 'A'),
        d.promTarget(quantile(requestBytes, '0.99'), 'p99', 'B'),
      ],
      pos(0, 27, 12, 9),
      { unit: 'bytes', fill: 0, description: 'Validated Lua JSON payload size; excludes Redis keys and RESP framing.' }
    ),

    d.timeSeries(
      'Pool and touched balances p95 / p99',
      [
        d.promTarget(quantile(poolSize, '0.95'), 'pool p95', 'A'),
        d.promTarget(quantile(poolSize, '0.99'), 'pool p99', 'B'),
        d.promTarget(quantile(touchedSize, '0.95'), 'touched p95', 'C'),
        d.promTarget(quantile(touchedSize, '0.99'), 'touched p99', 'D'),
      ],
      pos(12, 27, 12, 9),
      { unit: 'short', fill: 0, description: 'Full snapshot pool and distinct posting-target cardinality. Both histograms intentionally have no identifiers.' }
    ),

    d.row('Preflight and posting mix', 36),

    d.timeSeries(
      'CAS / preflight attempts',
      [d.promTarget(rate('balance_engine_cas_attempts_total'))],
      pos(0, 37, 12, 8),
      { unit: 'reqps', description: 'Preflight attempts, including receipt replay and normalization retries; NOSCRIPT fallback is not an additional attempt.' }
    ),

    d.timeSeries(
      'Validated postings by type',
      [d.promTarget(rate('balance_engine_postings_total', 'type'), '{{type}}')],
      pos(12, 37, 12, 8),
      { unit: 'reqps', legend: 'table', description: 'Requested postings by the six bounded posting types; not applied movements or generated companions.' }
    ),
  ]
)
