#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

cat > "$test_dir/allowlist.json" <<'EOF'
{"version":1,"entries":[
  {"package":"example.test/a","test":"TestChaosOne","reason":"Set CHAOS=1 to run","alternate_capability":"integration-chaos:CHAOS=1"},
  {"package":"example.test/b","test":"TestChaosTwo","reason":"Set CHAOS=1 to run","alternate_capability":"integration-chaos:CHAOS=1"}
]}
EOF

for shard in ledger-postgres ledger-mongodb-crm async-broker tracer lifecycle-migration chaos-capability; do
  mkdir -p "$test_dir/artifacts/integration-$shard-reports/integration-shards/$shard"
  printf 'package\ttest\toutcome\treason\talternate_capability\n' \
    > "$test_dir/artifacts/integration-$shard-reports/integration-shards/$shard/outcomes.tsv"
done
printf 'example.test/a\tTestChaosOne\tskipped\tSet CHAOS=1 to run\tintegration-chaos:CHAOS=1\n' \
  >> "$test_dir/artifacts/integration-lifecycle-migration-reports/integration-shards/lifecycle-migration/outcomes.tsv"
printf 'example.test/b\tTestChaosTwo\tskipped\tSet CHAOS=1 to run\tintegration-chaos:CHAOS=1\n' \
  >> "$test_dir/artifacts/integration-ledger-mongodb-crm-reports/integration-shards/ledger-mongodb-crm/outcomes.tsv"
printf 'example.test/a\tTestChaosOne\tpassed\t\t\nexample.test/b\tTestChaosTwo\tpassed\t\t\n' \
  >> "$test_dir/artifacts/integration-chaos-capability-reports/integration-shards/chaos-capability/outcomes.tsv"
cat > "$test_dir/artifacts/integration-chaos-capability-reports/integration-shards/chaos-capability/summary.json" <<'EOF'
{"shard":"chaos-capability","selected_test_count":2,"covered_test_count":2,"passed_test_count":2,"skipped_test_count":0,"failed_test_count":0,"missing_test_count":0,"unclassified_test_count":0,"uncovered_test_count":0,"flake_budget":0}
EOF

INTEGRATION_SKIP_ALLOWLIST="$test_dir/allowlist.json" \
  "$repo_root/scripts/verify-integration-capability.sh" "$test_dir/artifacts"

sed -i 's/TestChaosTwo\tpassed/TestChaosTwo\tskipped/' \
  "$test_dir/artifacts/integration-chaos-capability-reports/integration-shards/chaos-capability/outcomes.tsv"
status=0
INTEGRATION_SKIP_ALLOWLIST="$test_dir/allowlist.json" \
  "$repo_root/scripts/verify-integration-capability.sh" "$test_dir/artifacts" >/dev/null 2>&1 || status=$?
if [[ $status -eq 0 ]]; then
  echo "required capability verifier accepted a skipped alternate proof" >&2
  exit 1
fi

echo "verify-integration-capability tests passed"
