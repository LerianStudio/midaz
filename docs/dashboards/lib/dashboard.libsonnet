// Self-contained Grafana dashboard helpers for Midaz.
//
// This deliberately does NOT depend on github.com/grafana/grafonnet. That library
// requires jsonnet-bundler and a vendored tree of several thousand files, which is
// disproportionate for two dashboards. Everything here emits plain Grafana schema v39
// JSON and compiles with the `jsonnet` binary alone, so CI needs one static binary.

// Datasources resolve through dashboard variables rather than hard-coded UIDs, so the
// same JSON imports into any Grafana without an edit. The variables are declared first in
// `templating` below — Grafana resolves variables in list order, and the namespace and pod
// queries depend on the Prometheus one.
local datasource = {
  prometheus: { type: 'prometheus', uid: '${datasource}' },
  loki: { type: 'loki', uid: '${loki}' },
};

// Application metrics carry k8s_namespace_name; cAdvisor and kube-state-metrics carry
// namespace. Both hold the same value, so a single dashboard variable drives both. A
// query that filters application metrics by `namespace` silently returns nothing.
local selector = {
  // job="ledger" is load-bearing, not decoration. A namespace may also run the
  // pre-unification standalone midaz-crm deployment, which emits the same crm_protection_*
  // series as the unified binary. The pod variable is already ledger-only, but its All value
  // is the regex .*, which matches midaz-crm pods too — without the job filter the CRM
  // panels sum two separate services.
  app: 'job="ledger", k8s_namespace_name="$namespace", k8s_pod_name=~"$pod"',
  // Health checks dominate HTTP traffic in low-traffic environments — /readyz alone can be
  // ~80% of it — and would swamp every API panel, hiding real variation in business routes.
  api: 'job="ledger", k8s_namespace_name="$namespace", k8s_pod_name=~"$pod", http_route!~"/readyz|/health|/version|/"',
  // cAdvisor and kube-state-metrics carry no job label; adding one empties these panels.
  infra: 'namespace="$namespace"',
};

local gridPos(x, y, w, h) = { x: x, y: y, w: w, h: h };

{
  datasource: datasource,
  selector: selector,
  pos:: gridPos,

  // instant=true evaluates the expression once at the end of the range instead of at every
  // step. Stat panels need it: they reduce with lastNonNull, and a range query over a
  // series that only carries samples during a traffic burst reduces to a stale value or to
  // "No data".
  promTarget(expr, legend='', refId='A', instant=false):: {
    refId: refId,
    datasource: datasource.prometheus,
    expr: expr,
    legendFormat: legend,
    editorMode: 'code',
  } + (if instant then { instant: true, range: false } else { range: true }),

  lokiTarget(expr, legend='', refId='A'):: {
    refId: refId,
    datasource: datasource.loki,
    expr: expr,
    queryType: 'range',
  } + (if legend != '' then { legendFormat: legend } else {}),

  row(title, y):: {
    type: 'row',
    title: title,
    collapsed: false,
    panels: [],
    gridPos: gridPos(0, y, 24, 1),
  },

  timeSeries(title, targets, pos, opts={}):: (
    local o = {
      unit: 'short',
      description: '',
      stack: false,
      fill: 8,
      legend: 'list',
      min: null,
      max: null,
      datasource: datasource.prometheus,
    } + opts;
    {
      type: 'timeseries',
      title: title,
      description: o.description,
      datasource: o.datasource,
      gridPos: pos,
      targets: targets,
      options: {
        legend: {
          displayMode: o.legend,
          placement: 'bottom',
          calcs: if o.legend == 'table' then ['mean', 'max'] else [],
        },
        tooltip: { mode: 'multi', sort: 'desc' },
      },
      fieldConfig: {
        defaults: {
          unit: o.unit,
          color: { mode: 'palette-classic' },
          custom: {
            drawStyle: 'line',
            lineWidth: 2,
            fillOpacity: o.fill,
            showPoints: 'never',
            spanNulls: true,
            stacking: { mode: if o.stack then 'normal' else 'none', group: 'A' },
          },
        } + (if o.min != null then { min: o.min } else {})
          + (if o.max != null then { max: o.max } else {}),
        overrides: [],
      },
    }
  ),

  stat(title, expr, pos, opts={}):: (
    local o = {
      unit: 'short',
      description: '',
      decimals: null,
      steps: [{ color: 'text', value: null }],
    } + opts;
    {
      type: 'stat',
      title: title,
      description: o.description,
      datasource: datasource.prometheus,
      gridPos: pos,
      targets: [$.promTarget(expr, instant=true)],
      options: {
        reduceOptions: { calcs: ['lastNonNull'], fields: '', values: false },
        textMode: 'auto',
        colorMode: 'value',
        graphMode: 'area',
        justifyMode: 'auto',
      },
      fieldConfig: {
        defaults: {
          unit: o.unit,
          color: { mode: 'thresholds' },
          thresholds: { mode: 'absolute', steps: o.steps },
        } + (if o.decimals != null then { decimals: o.decimals } else {}),
        overrides: [],
      },
    }
  ),

  logs(title, expr, pos, description=''):: {
    type: 'logs',
    title: title,
    description: description,
    datasource: datasource.loki,
    gridPos: pos,
    targets: [$.lokiTarget(expr)],
    options: {
      showTime: true,
      wrapLogMessage: true,
      sortOrder: 'Descending',
      enableLogDetails: true,
      dedupStrategy: 'none',
      prettifyLogMessage: false,
    },
  },

  // Datasource variables come first so the namespace and pod queries can resolve against
  // them. Namespace and pod are sourced from a metric every midaz deployment emits, so the
  // dropdowns populate even when business traffic is idle.
  templating:: (
    local nsQuery = 'label_values(http_server_request_duration_seconds_count{job="ledger"}, k8s_namespace_name)';
    local podQuery = 'label_values(http_server_request_duration_seconds_count{job="ledger", k8s_namespace_name="$namespace"}, k8s_pod_name)';
    {
      list: [
        {
          name: 'datasource',
          label: 'Datasource',
          type: 'datasource',
          query: 'prometheus',
          refresh: 1,
          includeAll: false,
          multi: false,
          current: {},
        },
        {
          name: 'loki',
          label: 'Logs',
          type: 'datasource',
          query: 'loki',
          refresh: 1,
          includeAll: false,
          multi: false,
          current: {},
        },
        {
          name: 'namespace',
          label: 'Namespace',
          type: 'query',
          datasource: datasource.prometheus,
          refresh: 1,
          includeAll: false,
          multi: false,
          query: { qryType: 1, query: nsQuery, refId: 'namespace' },
          definition: nsQuery,
          current: {},
        },
        {
          name: 'pod',
          label: 'Pod',
          type: 'query',
          datasource: datasource.prometheus,
          refresh: 2,
          includeAll: true,
          multi: true,
          allValue: '.*',
          query: { qryType: 1, query: podQuery, refId: 'pod' },
          definition: podQuery,
          current: { text: ['All'], value: ['$__all'] },
        },
      ],
    }
  ),

  // Panel ids are assigned here by position so authors never hand-maintain them;
  // adding a panel mid-list renumbers deterministically instead of colliding.
  dashboard(uid, title, description, tags, panels):: {
    uid: uid,
    title: title,
    description: description,
    tags: tags,
    timezone: 'browser',
    schemaVersion: 39,
    version: 0,
    refresh: '1m',
    editable: true,
    graphTooltip: 1,
    time: { from: 'now-6h', to: 'now' },
    templating: $.templating,
    panels: std.mapWithIndex(function(i, p) p { id: i + 1 }, panels),
  },
}
