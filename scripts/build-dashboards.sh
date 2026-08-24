#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Compile every Grafana dashboard theme from Jsonnet to JSON.
#
# Themes live at docs/dashboards/<version>/<theme>/<theme>.libsonnet and compile to a
# sibling <theme>.json, which is a build artifact and is gitignored. Dashboards are scoped
# by midaz major version because the telemetry contract differs between them — v4 folds
# ledger, CRM and fees into one binary under job="ledger", while v3 runs them as separate
# services — so a single dashboard cannot address both.
#
# Requires only the `jsonnet` binary — the dashboards deliberately avoid the grafonnet
# library so there is nothing to vendor and no jsonnet-bundler step.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! command -v jsonnet >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: jsonnet not found on PATH.

Install it with:
  go install github.com/google/go-jsonnet/cmd/jsonnet@v0.22.0

and make sure "$(go env GOPATH)/bin" is on your PATH.
EOF
  exit 1
fi

built=0

for theme_dir in docs/dashboards/v*/*/; do
  theme="$(basename "$theme_dir")"
  src="${theme_dir}${theme}.libsonnet"

  # A version directory may hold only a README while its dashboards are still unwritten.
  [ -f "$src" ] || continue

  out="${theme_dir}${theme}.json"
  jsonnet "$src" -o "$out"
  echo "built ${out}"
  built=$((built + 1))
done

if [ "$built" -eq 0 ]; then
  echo "error: no dashboard themes found under docs/dashboards/v*/" >&2
  exit 1
fi

echo "compiled ${built} dashboard(s)"
