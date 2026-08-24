// Midaz Ledger — Golden Signals dashboard (RED + USE).
//
// Every query in this file was validated against live series in Mimir before being
// committed; see ../telemetry-dictionary.md for the primitive contract.

local d = import '../../lib/dashboard.libsonnet';

local api = d.selector.api;
local app = d.selector.app;
local infra = d.selector.infra;
local pos = d.pos;

local httpCount = 'http_server_request_duration_seconds_count';
local httpBucket = 'http_server_request_duration_seconds_bucket';

local quantile(q, extraGroup='') =
  local groupBy = if extraGroup == '' then 'le' else 'le, ' + extraGroup;
  'histogram_quantile(%s, sum by (%s) (rate(%s{%s}[$__rate_interval])))' % [q, groupBy, httpBucket, api];

d.dashboard(
  'midaz-v4-golden-signals',
  'Midaz · Ledger · Golden Signals (v4)',
  |||
    RED and USE signals for the midaz v4 unified ledger binary. Pick the target environment
    with the Namespace variable. Health-check routes are excluded from the API panels.
    Source of truth: this dashboard is generated from
    docs/dashboards/v4/golden-signals/golden-signals.libsonnet — edit there, not in the UI.
  |||,
  ['midaz', 'ledger', 'golden-signals', 'v4'],
  [
    d.row('Golden Signals — HTTP API (excludes /readyz, /health, /version)', 0),

    d.stat(
      'Throughput',
      'sum(rate(%s{%s}[$__rate_interval]))' % [httpCount, api],
      pos(0, 1, 6, 4),
      {
        unit: 'reqps',
        decimals: 2,
        description: 'Requests per second across business routes. Health checks are filtered out: /readyz alone can account for roughly 80% of traffic in a low-traffic environment and would mask any real variation in the API.',
      }
    ),

    d.stat(
      'Availability (non-5xx)',
      '100 * (1 - (sum(rate(%s{%s, http_response_status_code=~"5.."}[$__rate_interval])) OR vector(0)) / sum(rate(%s{%s}[$__rate_interval])))' % [httpCount, api, httpCount, api],
      pos(6, 1, 6, 4),
      {
        unit: 'percent',
        decimals: 3,
        description: 'Share of responses that are not 5xx. This is the API availability SLI.',
        steps: [
          { color: 'red', value: null },
          { color: 'orange', value: 99 },
          { color: 'green', value: 99.9 },
        ],
      }
    ),

    d.stat(
      '4xx error rate',
      '100 * (sum(rate(%s{%s, http_response_status_code=~"4.."}[$__rate_interval])) OR vector(0)) / sum(rate(%s{%s}[$__rate_interval]))' % [httpCount, api, httpCount, api],
      pos(12, 1, 6, 4),
      {
        unit: 'percent',
        decimals: 2,
        description: 'Client errors, kept separate from 5xx on purpose: a high 4xx rate usually means a broken contract or auth on the caller side, not a failing service.',
        steps: [
          { color: 'green', value: null },
          { color: 'orange', value: 5 },
          { color: 'red', value: 20 },
        ],
      }
    ),

    d.stat(
      'Latency p95',
      quantile('0.95'),
      pos(18, 1, 6, 4),
      {
        unit: 's',
        decimals: 3,
        description: 'Aggregate p95 across business routes.',
        steps: [
          { color: 'green', value: null },
          { color: 'orange', value: 0.5 },
          { color: 'red', value: 1 },
        ],
      }
    ),

    d.timeSeries(
      'Throughput by route',
      [d.promTarget('sum by (http_route) (rate(%s{%s}[$__rate_interval]))' % [httpCount, api], '{{http_route}}')],
      pos(0, 5, 12, 9),
      {
        unit: 'reqps',
        legend: 'table',
        description: 'Rate per route. Shows which endpoint carries the traffic and which one stopped receiving calls. A series with a blank legend is traffic that matched no registered route.',
      }
    ),

    d.timeSeries(
      'Latency percentiles',
      [
        d.promTarget(quantile('0.50'), 'p50', 'A'),
        d.promTarget(quantile('0.95'), 'p95', 'B'),
        d.promTarget(quantile('0.99'), 'p99', 'C'),
      ],
      pos(12, 5, 12, 9),
      {
        unit: 's',
        fill: 0,
        description: 'p50, p95 and p99. A wide gap between p50 and p99 points to a long tail — a few slow requests rather than general degradation.',
      }
    ),

    d.timeSeries(
      'Responses by status code',
      [d.promTarget('sum by (http_response_status_code) (rate(%s{%s}[$__rate_interval]))' % [httpCount, api], '{{http_response_status_code}}')],
      pos(0, 14, 12, 8),
      {
        unit: 'reqps',
        stack: true,
        legend: 'table',
        description: 'Status distribution, stacked so the error share can be read against total volume.',
      }
    ),

    d.timeSeries(
      'p95 by route (top 10)',
      [d.promTarget('topk(10, %s)' % quantile('0.95', 'http_route'), '{{http_route}}')],
      pos(12, 14, 12, 8),
      {
        unit: 's',
        fill: 0,
        legend: 'table',
        description: 'The ten slowest routes at p95. Starting point for investigating a performance regression.',
      }
    ),

    d.row('Readiness & Dependencies', 22),

    d.timeSeries(
      "Dependency health (fraction 'up')",
      [d.promTarget(
        'sum by (checker) (rate(readyz_check_status_total{%s, status="up"}[$__rate_interval])) / sum by (checker) (rate(readyz_check_status_total{%s}[$__rate_interval]))' % [app, app],
        '{{checker}}'
      )],
      pos(0, 23, 12, 9),
      {
        unit: 'percentunit',
        min: 0,
        max: 1,
        fill: 0,
        legend: 'table',
        description: "Fraction of 'up' checks per dependency: mongo, mongo_crm, mongo_fees, mongo_onboarding, mongo_transaction, postgres_onboarding, postgres_transaction, rabbitmq, redis, vault. 1 means healthy. Vault reports 'n/a' while the KMS runs in legacy mode, so it sits below 1 without being a failure.",
      }
    ),

    d.timeSeries(
      'Readyz check p95 by dependency',
      [d.promTarget(
        'histogram_quantile(0.95, sum by (le, checker) (rate(readyz_check_duration_ms_milliseconds_bucket{%s}[$__rate_interval])))' % app,
        '{{checker}}'
      )],
      pos(12, 23, 12, 9),
      {
        unit: 'ms',
        fill: 0,
        legend: 'table',
        description: 'p95 duration of each health check. A dependency slowing down here precedes timeouts on real requests.',
      }
    ),

    d.timeSeries(
      'Dependency p95 by span',
      [d.promTarget(
        // The emitted PromQL must contain \\. — two characters — not \. as it looks.
        // A label-matcher value is a Go-style string literal, so PromQL unescapes it before
        // the regex engine sees it: \\. becomes \. which anchors the dot. A single \. in the
        // emitted query is not a lax escape, it is a hard parse error —
        // `unknown escape sequence U+002E '.'` — and the panel returns nothing at all.
        // Jsonnet halves backslashes too, hence \\\\ in this source for \\ on the wire.
        'histogram_quantile(0.95, sum by (le, span_name) (rate(duration_milliseconds_bucket{%s, span_name=~"postgres\\\\..*|mongo\\\\..*|mongodb\\\\..*|redis\\\\..*"}[$__rate_interval])))' % app,
        '{{span_name}}'
      )],
      pos(0, 32, 12, 9),
      {
        unit: 'ms',
        fill: 0,
        legend: 'table',
        description: 'p95 per I/O operation, derived from spans by the spanmetrics connector. Shows whether latency sits in Postgres, Mongo or Redis.',
      }
    ),

    d.timeSeries(
      'Calls by span',
      [d.promTarget('sum by (span_name) (rate(calls_total{%s}[$__rate_interval]))' % app, '{{span_name}}')],
      pos(12, 32, 12, 9),
      {
        unit: 'reqps',
        legend: 'table',
        description: 'Call volume per span, including I/O and auth. A jump in mongo.ping or redis.get_balance_sync_keys indicates a worker loop.',
      }
    ),

    d.row('Resources', 41),

    d.timeSeries(
      'Process CPU (%)',
      [d.promTarget('system_cpu_usage_percentage{%s}' % app, '{{k8s_pod_name}}')],
      pos(0, 42, 8, 8),
      { unit: 'percent', fill: 4, description: 'CPU usage reported by the lib-observability runtime, per pod.' }
    ),

    d.timeSeries(
      'Process memory (%)',
      [d.promTarget('system_mem_usage_percentage{%s}' % app, '{{k8s_pod_name}}')],
      pos(8, 42, 8, 8),
      { unit: 'percent', fill: 4, description: 'Memory usage reported by the lib-observability runtime, per pod.' }
    ),

    d.timeSeries(
      'Container working set',
      [d.promTarget('sum by (pod) (container_memory_working_set_bytes{%s, container!="", container!="POD"})' % infra, '{{pod}}')],
      pos(16, 42, 8, 8),
      {
        unit: 'bytes',
        fill: 4,
        description: 'Real container memory from cAdvisor. Compare against the limit: this is the value that triggers an OOMKill, not the runtime percentage.',
      }
    ),

    d.timeSeries(
      'CPU throttling',
      [d.promTarget(
        'sum by (pod) (rate(container_cpu_cfs_throttled_periods_total{%s, container!=""}[$__rate_interval])) / clamp_min(sum by (pod) (rate(container_cpu_cfs_periods_total{%s, container!=""}[$__rate_interval])), 1)' % [infra, infra],
        '{{pod}}'
      )],
      pos(0, 50, 12, 8),
      {
        unit: 'percentunit',
        min: 0,
        fill: 4,
        description: 'Fraction of CFS periods in which the container was throttled. Anything above zero already adds latency, even when average CPU looks low.',
      }
    ),

    d.timeSeries(
      'Container restarts (in window)',
      [d.promTarget('sum by (pod) (increase(kube_pod_container_status_restarts_total{%s}[$__range]))' % infra, '{{pod}}')],
      pos(12, 50, 12, 8),
      {
        fill: 4,
        description: 'Restarts accumulated over the selected period. The *-migrate-* jobs show up here and are expected.',
      }
    ),

    d.row('Logs', 58),

    d.timeSeries(
      'Log volume by level',
      [d.lokiTarget(
        'sum by (level) (count_over_time({k8s_namespace_name="$namespace", service_name=~"ledger|midaz-crm"}[$__auto]))',
        '{{level}}'
      )],
      pos(0, 59, 8, 10),
      {
        stack: true,
        datasource: d.datasource.loki,
        description: 'Line count per level. Spikes in ERROR or WARN correlate with the 5xx spikes above.',
      }
    ),

    d.logs(
      'ERROR and WARN logs',
      '{k8s_namespace_name="$namespace", service_name=~"ledger|midaz-crm", level=~"ERROR|WARN"}',
      pos(8, 59, 16, 10),
      "ERROR and WARN lines from ledger and midaz-crm. 'level' is a Loki stream label (OTLP ingestion), so this filter is cheap."
    ),
  ]
)
