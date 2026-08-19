#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

mkdir -p "$test_dir/bin" "$test_dir/calls"
cat > "$test_dir/bin/gotestsum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

events=
junit=
for arg in "$@"; do
  case $arg in
    --jsonfile=*) events=${arg#--jsonfile=} ;;
    --junitfile=*) junit=${arg#--junitfile=} ;;
  esac
done

[[ -n $events && -n $junit ]]
[[ -n ${MIDAZ_SHARD_JOB_SELECTION_FILE:-} ]]
mkdir -p "$(dirname "$events")"
: > "$events"
while IFS= read -r test_name; do
  [[ -n $test_name ]] || continue
  printf '{"Action":"run","Package":"%s","Test":"%s"}\n' "$MIDAZ_SHARD_JOB_PACKAGE" "$test_name" >> "$events"
  printf '{"Action":"pass","Package":"%s","Test":"%s"}\n' "$MIDAZ_SHARD_JOB_PACKAGE" "$test_name" >> "$events"
done < "$MIDAZ_SHARD_JOB_SELECTION_FILE"
printf '<testsuite/>\n' > "$junit"
printf '%s\t%s\t%s\t%s\t%s\n' \
  "$MIDAZ_SHARD_JOB_MODE" "$MIDAZ_SHARD_JOB_PACKAGE" \
  "$TESTCONTAINERS_SESSION_ID" "$INTEGRATION_PACKAGE_PARALLELISM" "$*" \
  > "$FAKE_CALLS_DIR/$MIDAZ_SHARD_JOB_INDEX.tsv"

if [[ ${FAKE_FAIL_PACKAGE:-} == "$MIDAZ_SHARD_JOB_PACKAGE" ]]; then
  exit 17
fi
EOF
chmod +x "$test_dir/bin/gotestsum"

cat > "$test_dir/tracer-plan.tsv" <<'EOF'
tracer	parallel	example.test/tracer-worker	TestWorkerOne
tracer	parallel	example.test/tracer-cache	TestCacheOne
tracer	serial	example.test/tracer-journey	TestJourneyOne
tracer	serial	example.test/tracer-journey	TestJourneyTwo
EOF

PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  TESTCONTAINERS_SESSION_ID=owner-tracer \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/tracer-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=3 \
  INTEGRATION_SHUFFLE_SEED=417 \
  TEST_REPORTS_DIR="$test_dir/reports" \
  "$repo_root/scripts/run-integration-shard.sh" tracer

grep -q $'parallel\texample.test/tracer-worker\towner-tracer\t2\t.*-parallel 3.*-shuffle=417' "$test_dir/calls/001.tsv"
grep -q $'parallel\texample.test/tracer-cache\towner-tracer\t2\t.*-parallel 3.*-shuffle=417' "$test_dir/calls/002.tsv"
grep -q $'serial\texample.test/tracer-journey\towner-tracer\t1\t.*-parallel 1.*-shuffle=417' "$test_dir/calls/003.tsv"
grep -q '"selected_test_count":4' "$test_dir/reports/tracer/summary.json"
grep -q '"failed_jobs":0' "$test_dir/reports/tracer/summary.json"
test -s "$test_dir/reports/tracer/selection.sha256"
test -s "$test_dir/reports/tracer/jobs/003/events.json"
test -s "$test_dir/reports/tracer/jobs/003/junit.xml"

status=0
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_FAIL_PACKAGE=example.test/tracer-cache \
  TESTCONTAINERS_SESSION_ID=owner-tracer-failure \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/tracer-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=2 \
  TEST_REPORTS_DIR="$test_dir/failure-reports" \
  "$repo_root/scripts/run-integration-shard.sh" tracer || status=$?
if [[ $status -eq 0 ]]; then
  echo "failed package produced a passing shard" >&2
  exit 1
fi
grep -q '"failed_jobs":1' "$test_dir/failure-reports/tracer/summary.json"
grep -q 'example.test/tracer-cache' "$test_dir/failure-reports/tracer/failures.tsv"
test -s "$test_dir/failure-reports/tracer/jobs/001/events.json"
test -s "$test_dir/failure-reports/tracer/jobs/003/events.json"

status=0
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  TESTCONTAINERS_SESSION_ID=owner-invalid \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/tracer-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=5 \
  TEST_REPORTS_DIR="$test_dir/invalid-reports" \
  "$repo_root/scripts/run-integration-shard.sh" tracer || status=$?
if [[ $status -ne 2 ]]; then
  echo "package parallelism above cap returned $status, want 2" >&2
  exit 1
fi

echo "run-integration-shard tests passed"
