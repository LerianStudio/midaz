{# ================================================================== #}
{# Reporter Template Engine — Feature Showcase                       #}
{# Exercises 17 of 19 custom features in a single self-documenting  #}
{# HTML report that shows raw data, computed results, and            #}
{# explanations for each feature.                                    #}
{# Datasources: midaz_onboarding (PG), midaz_transaction (PG)       #}
{#                                                                   #}
{# Blocked by TASK-005 (template validator bug):                     #}
{#   - Filter #16: sum (pipe) at collection level                    #}
{#   - Filter #17: count (pipe) at collection level                  #}
{# Also blocked (pongo2 builtins): forloop.Last, forloop.Counter     #}
{# See: docs/reporter/incidents/2026-03-12_template-validator-       #}
{#      misparses-pongo2-filters.md                                  #}
{# ================================================================== #}
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Reporter Engine — Feature Showcase</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1a1a2e;background:#f8f9fa;padding:2rem;max-width:1200px;margin:0 auto}
h1{font-size:1.8rem;margin-bottom:.5rem}
h2{font-size:1.4rem;margin:2rem 0 1rem;border-bottom:2px solid #e0e0e0;padding-bottom:.5rem}
h3{font-size:1.1rem;margin:1.5rem 0 .5rem}
p{margin:.4rem 0;line-height:1.5}
table{width:100%;border-collapse:collapse;margin:.5rem 0 1.5rem;font-size:.9rem}
th,td{padding:.4rem .6rem;text-align:left;border:1px solid #dee2e6}
th{background:#e9ecef;font-weight:600}
tr:nth-child(even){background:#f8f9fa}
.meta{color:#6c757d;font-size:.9rem}
.card{background:#fff;border:1px solid #dee2e6;border-radius:8px;padding:1.25rem;margin:1rem 0}
.card h4{margin-bottom:.6rem}
.badge{display:inline-block;padding:.1rem .45rem;border-radius:4px;font-size:.75rem;font-weight:600;text-transform:uppercase;margin-right:.4rem}
.b-tag{background:#d4edda;color:#155724}
.b-filter{background:#cce5ff;color:#004085}
.b-func{background:#fff3cd;color:#856404}
.b-blocked{background:#f8d7da;color:#721c24}
.result{background:#f0fff4;border-left:3px solid #28a745;padding:.5rem 1rem;margin-top:.5rem}
.result .label{font-weight:600}
.blocked{background:#fff5f5;border-left:3px solid #dc3545;padding:.5rem 1rem;margin-top:.5rem;color:#721c24}
.combo{background:#fff;border:1px solid #dee2e6;border-radius:8px;padding:1.25rem;margin:1rem 0}
.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:1rem;margin-top:1rem}
.grid-item{background:#fff;border:1px solid #dee2e6;border-radius:8px;padding:1rem;text-align:center}
.grid-item .val{font-size:1.5rem;font-weight:700;color:#155724}
.grid-item .lbl{font-size:.8rem;color:#6c757d;margin-top:.3rem}
</style>
</head>
<body>

<header>
<h1>Reporter Template Engine — Feature Showcase</h1>
<p class="meta">Generated: {% date_time "dd/MM/YYYY HH:mm:ss" %} | Datasources: midaz_onboarding, midaz_transaction</p>
<p>This report exercises <strong>17 of 19 custom features</strong> of the template engine. Each section shows raw source data, the computed result, and why the feature exists.</p>
<p class="meta"><strong>Not exercised (blocked by TASK-005):</strong> sum pipe (#16), count pipe (#17) — template validator misparses collection-level pipe filters. Also blocked: forloop builtins (forloop.Last, forloop.Counter).</p>
</header>

{# ================================================================ #}
{# SECTION 1: SOURCE DATA                                          #}
{# ================================================================ #}

<h2>Section 1 — Source Data</h2>
<p>Tables below show the raw data used as input for all feature demonstrations.</p>

<h3>1.1 Accounts (midaz_onboarding.account — 7 rows)</h3>
<table>
<thead><tr><th>Name</th><th>Alias</th><th>Type</th><th>Asset</th><th>Balance</th><th>Status</th></tr></thead>
<tbody>
{%- for acc in midaz_onboarding.account %}
<tr><td>{{ acc.name }}</td><td>{{ acc.alias }}</td><td>{{ acc.type }}</td><td>{{ acc.asset_code }}</td><td>{{ acc.balance }}</td><td>{{ acc.status }}</td></tr>
{%- endfor %}
</tbody>
</table>

<h3>1.2 Organizations (midaz_onboarding.organization)</h3>
<table>
<thead><tr><th>Name</th><th>Legal Document</th><th>Country</th><th>Status</th></tr></thead>
<tbody>
{%- for org in midaz_onboarding.organization %}
<tr><td>{{ org.name }}</td><td>{{ org.legal_document }}</td><td>{{ org.country }}</td><td>{{ org.status }}</td></tr>
{%- endfor %}
</tbody>
</table>

<h3>1.3 Operations (midaz_transaction.operation — 3 rows)</h3>
<table>
<thead><tr><th>Account ID</th><th>Route</th><th>Balance After</th><th>Created At</th></tr></thead>
<tbody>
{%- for op in midaz_transaction.operation %}
<tr><td>{{ op.account_id }}</td><td>{{ op.route }}</td><td>{{ op.available_balance_after }}</td><td>{{ op.created_at }}</td></tr>
{%- endfor %}
</tbody>
</table>

<h3>1.4 Operation Routes (midaz_transaction.operation_route — 2 rows)</h3>
<table>
<thead><tr><th>ID</th><th>Code</th></tr></thead>
<tbody>
{%- for r in midaz_transaction.operation_route %}
<tr><td>{{ r.id }}</td><td>{{ r.code }}</td></tr>
{%- endfor %}
</tbody>
</table>

{# ================================================================ #}
{# SECTION 2: TAGS (10 features)                                   #}
{# ================================================================ #}

<h2>Section 2 — Tags (10 Features)</h2>

<div class="card" id="feat-date-time">
<h4><span class="badge b-tag">Tag</span> #1 — date_time</h4>
<p>Renders current date/time in a specified format. Every financial report needs a generation timestamp for audit trail and regulatory compliance.</p>
<div class="result">
<span class="label">Full timestamp:</span> {% date_time "dd/MM/YYYY HH:mm:ss" %}<br>
<span class="label">Date only:</span> {% date_time "YYYY-MM-dd" %}<br>
<span class="label">Compact:</span> {% date_time "YYYYMMdd" %}
</div>
</div>

<div class="card" id="feat-sum-by">
<h4><span class="badge b-tag">Tag</span> #2 — sum_by</h4>
<p>Sums a numeric field across all rows in a collection. Uses decimal arithmetic to avoid floating-point rounding errors — critical for balance sheets and regulatory totals.</p>
<div class="result">
<span class="label">Sum of operation balances:</span> {% sum_by midaz_transaction.operation by "available_balance_after" %}<br>
<span class="label">Sum of account balances:</span> {% sum_by midaz_onboarding.account by "balance" %}
</div>
</div>

<div class="card" id="feat-count-by">
<h4><span class="badge b-tag">Tag</span> #3 — count_by</h4>
<p>Counts rows in a collection, optionally filtered by a condition. Used for report headers ("showing X of Y records") and regulatory record counts.</p>
<div class="result">
<span class="label">Total accounts:</span> {% count_by midaz_onboarding.account %}<br>
<span class="label">Active accounts:</span> {% count_by midaz_onboarding.account if status == "active" %}<br>
<span class="label">Suspended accounts:</span> {% count_by midaz_onboarding.account if status == "suspended" %}
</div>
</div>

<div class="card" id="feat-avg-by">
<h4><span class="badge b-tag">Tag</span> #4 — avg_by</h4>
<p>Calculates the average of a numeric field. Used in portfolio analysis and KPI dashboards to show mean values across accounts or transactions.</p>
<div class="result">
<span class="label">Average account balance:</span> {% avg_by midaz_onboarding.account by "balance" %}<br>
<span class="label">Average operation balance:</span> {% avg_by midaz_transaction.operation by "available_balance_after" %}
</div>
</div>

<div class="card" id="feat-min-by">
<h4><span class="badge b-tag">Tag</span> #5 — min_by</h4>
<p>Finds the minimum value in a field. Critical for risk reports showing lowest balances, minimum transaction amounts, or compliance thresholds.</p>
<div class="result">
<span class="label">Minimum account balance:</span> {% min_by midaz_onboarding.account by "balance" %}
</div>
</div>

<div class="card" id="feat-max-by">
<h4><span class="badge b-tag">Tag</span> #6 — max_by</h4>
<p>Finds the maximum value in a field. Used for exposure limits, highest transaction detection, and portfolio concentration analysis.</p>
<div class="result">
<span class="label">Maximum account balance:</span> {% max_by midaz_onboarding.account by "balance" %}
</div>
</div>

<div class="card" id="feat-calc">
<h4><span class="badge b-tag">Tag</span> #7 — calc</h4>
<p>Inline arithmetic with variables, parentheses, and operator precedence (+, -, *, /, **). Enables derived metrics directly in templates without pre-computation.</p>
<div class="result">
<span class="label">First account + 5% interest:</span> {% calc midaz_onboarding.account.0.balance * 1.05 %}<br>
<span class="label">Average of first two (manual):</span> {% calc (midaz_onboarding.account.0.balance + midaz_onboarding.account.1.balance) / 2 %}<br>
<span class="label">Power: 100 ** 2 =</span> {% calc 100 ** 2 %}
</div>
</div>

<div class="card" id="feat-last-item-by-group">
<h4><span class="badge b-tag">Tag</span> #8 — last_item_by_group</h4>
<p>Groups rows by key and keeps only the latest per group (by date). Essential for "latest balance per account" or "last operation per route" in regulatory filings like CADOC 4111.</p>
{% last_item_by_group midaz_transaction.operation group_by "route" order_by "created_at" as latest_ops %}
<div class="result">
<span class="label">Latest operation per route:</span>
<table>
<thead><tr><th>Route</th><th>Balance After</th><th>Created At</th></tr></thead>
<tbody>
{%- for op in latest_ops %}
<tr><td>{{ op.route }}</td><td>{{ op.available_balance_after }}</td><td>{{ op.created_at }}</td></tr>
{%- endfor %}
</tbody>
</table>
</div>
</div>

<div class="card" id="feat-counter">
<h4><span class="badge b-tag">Tag</span> #9 — counter</h4>
<p>Render-scoped named counter that increments during iteration. Used in regulatory documents (CADOC, DIMP) where line counts per category are required. Thread-safe — each render gets isolated storage.</p>
{%- for acc in midaz_onboarding.account %}
{%- if acc.type == "deposit" %}{% counter "deposit" %}{% endif %}
{%- if acc.type == "savings" %}{% counter "savings" %}{% endif %}
{%- if acc.type == "expense" %}{% counter "expense" %}{% endif %}
{%- if acc.type == "investment" %}{% counter "investment" %}{% endif %}
{%- counter "all_accounts" %}
{%- endfor %}
<div class="result">
<span class="label">Counters accumulated during iteration over 7 accounts.</span> (See #10 for display.)
</div>
</div>

<div class="card" id="feat-counter-show">
<h4><span class="badge b-tag">Tag</span> #10 — counter_show</h4>
<p>Displays the accumulated value of one or more named counters. Supports summing multiple counters in a single call — useful for category subtotals and grand totals.</p>
<div class="result">
<span class="label">Deposit:</span> {% counter_show "deposit" %}<br>
<span class="label">Savings:</span> {% counter_show "savings" %}<br>
<span class="label">Expense:</span> {% counter_show "expense" %}<br>
<span class="label">Investment:</span> {% counter_show "investment" %}<br>
<span class="label">All:</span> {% counter_show "all_accounts" %}<br>
<span class="label">Deposit + Savings (combined):</span> {% counter_show "deposit" "savings" %}
</div>
</div>

{# ================================================================ #}
{# SECTION 3: FILTERS (7 features — 5 exercised, 2 blocked)        #}
{# ================================================================ #}

<h2>Section 3 — Filters (5 of 7 Exercised)</h2>

<div class="card" id="feat-slice">
<h4><span class="badge b-filter">Filter</span> #11 — slice</h4>
<p>Extracts a substring by index range. Essential for Brazilian regulatory reports where CNPJ prefixes (first 8 digits) identify entities in CCS/CADOC filings.</p>
<div class="result">
<span class="label">Full CNPJ:</span> {{ midaz_onboarding.organization.0.legal_document }}<br>
<span class="label">Prefix (first 8):</span> {{ midaz_onboarding.organization.0.legal_document|slice:":8" }}<br>
<span class="label">Middle (4:10):</span> {{ midaz_onboarding.organization.0.legal_document|slice:"4:10" }}
</div>
</div>

<div class="card" id="feat-percent-of">
<h4><span class="badge b-filter">Filter</span> #12 — percent_of</h4>
<p>Calculates (value/denominator)*100 with 2 decimal places and % suffix. Used for portfolio allocation, compliance thresholds, and concentration reports.</p>
<div class="result">
<span class="label">First account (250K) as % of 1M:</span> {{ midaz_onboarding.account.0.balance|percent_of:1000000 }}<br>
<span class="label">Second account (500K) as % of 1M:</span> {{ midaz_onboarding.account.1.balance|percent_of:1000000 }}
</div>
</div>

<div class="card" id="feat-strip-zeros">
<h4><span class="badge b-filter">Filter</span> #13 — strip_zeros</h4>
<p>Removes trailing zeros from decimals. Financial systems store NUMERIC(18,2) but reports should show clean numbers. Uses decimal arithmetic to preserve precision.</p>
<div class="result">
<span class="label">Account balance (raw):</span> {{ midaz_onboarding.account.2.balance }}<br>
<span class="label">Account balance (stripped):</span> {{ midaz_onboarding.account.2.balance|strip_zeros }}<br>
<span class="label">Operation balance (stripped):</span> {{ midaz_transaction.operation.0.available_balance_after|strip_zeros }}
</div>
</div>

<div class="card" id="feat-replace">
<h4><span class="badge b-filter">Filter</span> #14 — replace</h4>
<p>Replaces all occurrences of a substring. Used for formatting documents (stripping separators from CEP/CNPJ), localizing decimal separators, and display normalization.</p>
<div class="result">
<span class="label">Original:</span> {{ midaz_onboarding.account.0.name }}<br>
<span class="label">Shortened:</span> {{ midaz_onboarding.account.0.name|replace:"Account:Acct" }}<br>
<span class="label">Org name:</span> {{ midaz_onboarding.organization.0.name|replace:"Corp:Corporation" }}
</div>
</div>

<div class="card" id="feat-where">
<h4><span class="badge b-filter">Filter</span> #15 — where</h4>
<p>Filters an array by field value, returning only matching rows. Enables in-template data segmentation without modifying queries — one template, multiple views of the same dataset.</p>
<div class="result">
<span class="label">Active accounts only:</span>
<table>
<thead><tr><th>Name</th><th>Type</th><th>Balance</th></tr></thead>
<tbody>
{%- for acc in midaz_onboarding.account|where:"status:active" %}
<tr><td>{{ acc.name }}</td><td>{{ acc.type }}</td><td>{{ acc.balance }}</td></tr>
{%- endfor %}
</tbody>
</table>
</div>
</div>

<div class="card" id="feat-sum-pipe">
<h4><span class="badge b-blocked">Blocked</span> #16 — sum (pipe)</h4>
<p>Sums a numeric field across array items. Unlike the sum_by tag, this pipe filter can be chained with where for filtered aggregations. Uses decimal arithmetic.</p>
<div class="blocked">
Not exercised — blocked by TASK-005. Template validator misparses collection-level pipe filters
(e.g., <code>collection|sum:"field"</code>) as table names, causing TPL-0030.
Use <code>sum_by</code> tag (#2) as equivalent.
</div>
</div>

<div class="card" id="feat-count-pipe">
<h4><span class="badge b-blocked">Blocked</span> #17 — count (pipe)</h4>
<p>Counts items matching a field value. Pipe-based alternative to count_by tag, enabling inline counts and filter chain composition.</p>
<div class="blocked">
Not exercised — blocked by TASK-005. Template validator misparses collection-level pipe filters
(e.g., <code>collection|count:"field:value"</code>) as table names, causing TPL-0030.
Use <code>count_by</code> tag (#3) as equivalent.
</div>
</div>

{# ================================================================ #}
{# SECTION 4: FUNCTIONS (2 features)                               #}
{# ================================================================ #}

<h2>Section 4 — Functions (2 Features)</h2>

<div class="card" id="feat-filter-func">
<h4><span class="badge b-func">Function</span> #18 — filter()</h4>
<p>Filters a collection by field value and returns matching rows. Commonly used with [0] index to extract a single match — e.g., finding a specific account's balance in a cross-table join.</p>
{%- with route_ops = filter(midaz_transaction.operation, "route", midaz_transaction.operation_route.0.id) %}
<div class="result">
<span class="label">Operations matching first route:</span>
<table>
<thead><tr><th>Account ID</th><th>Balance After</th><th>Created At</th></tr></thead>
<tbody>
{%- for op in route_ops %}
<tr><td>{{ op.account_id }}</td><td>{{ op.available_balance_after }}</td><td>{{ op.created_at }}</td></tr>
{%- endfor %}
</tbody>
</table>
</div>
{%- endwith %}
</div>

<div class="card" id="feat-contains">
<h4><span class="badge b-func">Function</span> #19 — contains()</h4>
<p>Case-insensitive substring check returning boolean. Used for conditional rendering — highlighting specific organizations, flagging accounts by name pattern, or routing sections based on data content.</p>
<div class="result">
<span class="label">First org contains "acme":</span> {% if contains(midaz_onboarding.organization.0.name, "acme") %}YES — matched "{{ midaz_onboarding.organization.0.name }}"{% else %}NO{% endif %}<br>
<span class="label">First org contains "xyz":</span> {% if contains(midaz_onboarding.organization.0.name, "xyz") %}YES{% else %}NO — not found in "{{ midaz_onboarding.organization.0.name }}"{% endif %}
</div>
</div>

{# ================================================================ #}
{# SECTION 5: ADVANCED COMBINATIONS                                #}
{# ================================================================ #}

<h2>Section 5 — Advanced Combinations</h2>
<p>Features compose naturally. Below are cross-feature integrations demonstrating real-world financial report patterns.</p>

<div class="combo">
<h4>5.1 Conditional rendering with contains()</h4>
<p>Iterates organizations and highlights those containing "Corp" in the name.</p>
<div class="result">
{%- for org in midaz_onboarding.organization %}
{% if contains(org.name, "corp") %}<strong>&#x2605; {{ org.name }}</strong>{% else %}{{ org.name }}{% endif %}
{%- endfor %}
</div>
</div>

<div class="combo">
<h4>5.2 Summary Dashboard</h4>
<p>Combining multiple tag features into a consolidated financial overview.</p>
<div class="grid">
<div class="grid-item">
  <div class="val">{% count_by midaz_onboarding.account %}</div>
  <div class="lbl">Total Accounts</div>
</div>
<div class="grid-item">
  <div class="val">{% sum_by midaz_onboarding.account by "balance" %}</div>
  <div class="lbl">Total Balance</div>
</div>
<div class="grid-item">
  <div class="val">{% min_by midaz_onboarding.account by "balance" %}</div>
  <div class="lbl">Min Balance</div>
</div>
<div class="grid-item">
  <div class="val">{% max_by midaz_onboarding.account by "balance" %}</div>
  <div class="lbl">Max Balance</div>
</div>
</div>
</div>

</body>
</html>