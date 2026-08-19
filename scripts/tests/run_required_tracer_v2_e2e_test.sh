#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
mkdir -p "$test_dir/bin"

cat > "$test_dir/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case ${FAKE_LEDGER_MODE:-legacy} in
  v2)
    printf '%s\n' '{"status":"healthy","tracer_outcome_mode":"ledger_outcome_v2"}'
    ;;
  legacy)
    printf '%s\n' '{"status":"healthy","tracer_outcome_mode":"legacy"}'
    ;;
  unavailable)
    exit 7
    ;;
esac
EOF
chmod +x "$test_dir/bin/curl"

cat > "$test_dir/bin/gotestsum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
json_file=
junit_file=
while [[ $# -gt 0 ]]; do
  case $1 in
    --jsonfile) json_file=$2; shift 2 ;;
    --junitfile) junit_file=$2; shift 2 ;;
    *) shift ;;
  esac
done
printf '%s\n' called > "$FAKE_GOTESTSUM_MARKER"
printf '%s\n' '{"Action":"pass","Test":"TestTracerOutcomeV2LedgerToTracerAndLostACK"}' > "$json_file"
printf '%s\n' '<testsuite name="fake"/>' > "$junit_file"
EOF
chmod +x "$test_dir/bin/gotestsum"

marker="$test_dir/gotestsum-called"
status=0
PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/legacy" FAKE_LEDGER_MODE=legacy \
  FAKE_GOTESTSUM_MARKER="$marker" E2E_TRACER_V2_WALL_TIMEOUT=5s \
  "$repo_root/scripts/run-required-tracer-v2-e2e.sh" || status=$?
if [[ $status -eq 0 ]]; then
  echo "Tracer V2 E2E accepted a running Ledger in legacy outcome mode" >&2
  exit 1
fi
if [[ -e $marker ]]; then
  echo "Tracer V2 E2E started tests before rejecting legacy Ledger mode" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/legacy/ledger-tracer-v2-e2e-timing.json"

PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/v2" FAKE_LEDGER_MODE=v2 \
  FAKE_GOTESTSUM_MARKER="$marker" E2E_TRACER_V2_WALL_TIMEOUT=5s \
  "$repo_root/scripts/run-required-tracer-v2-e2e.sh"
test -s "$marker"
grep -q '"status":"passed"' "$test_dir/v2/ledger-tracer-v2-e2e-timing.json"

echo "run-required-tracer-v2-e2e tests passed"
