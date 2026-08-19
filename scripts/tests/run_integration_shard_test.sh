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
if [[ ${FAKE_RESET_WAVE_CLEANUP:-0} == 1 ]]; then
  rm -f "$FAKE_DOCKER_STATE_DIR/wave.removed"
fi
mkdir -p "$(dirname "$events")"
: > "$events"
while IFS= read -r test_name; do
  [[ -n $test_name ]] || continue
  printf '{"Action":"run","Package":"%s","Test":"%s"}\n' "$MIDAZ_SHARD_JOB_PACKAGE" "$test_name" >> "$events"
  printf '{"Action":"pass","Package":"%s","Test":"%s"}\n' "$MIDAZ_SHARD_JOB_PACKAGE" "$test_name" >> "$events"
done < "$MIDAZ_SHARD_JOB_SELECTION_FILE"
printf '<testsuite/>\n' > "$junit"
printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
  "$MIDAZ_SHARD_JOB_MODE" "$MIDAZ_SHARD_JOB_PACKAGE" \
  "$TESTCONTAINERS_SESSION_ID" "$INTEGRATION_PACKAGE_PARALLELISM" \
  "$RYUK_RECONNECTION_TIMEOUT" "$*" \
  > "$FAKE_CALLS_DIR/$MIDAZ_SHARD_JOB_INDEX.tsv"

if [[ ${FAKE_FAIL_PACKAGE:-} == "$MIDAZ_SHARD_JOB_PACKAGE" ]]; then
  exit 17
fi
EOF
chmod +x "$test_dir/bin/gotestsum"

cat > "$test_dir/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case ${1:-} in
  ps)
    cleanup_key=${MIDAZ_SHARD_JOB_INDEX:-wave}
    state_file="$FAKE_DOCKER_STATE_DIR/${cleanup_key}.removed"
    if [[ ! -e $state_file ]]; then
      printf 'deadbeefdead\tjob-%s-postgres\tpostgres:17-alpine\torg.testcontainers.sessionId=%s\n' \
        "$cleanup_key" "$TESTCONTAINERS_SESSION_ID"
    fi
    printf 'feedfacefeed\treaper_%s\ttestcontainers/ryuk:0.14.0\torg.testcontainers.ryuk=true,org.testcontainers.sessionId=%s\n' \
      "$TESTCONTAINERS_SESSION_ID" "$TESTCONTAINERS_SESSION_ID"
    ;;
  rm)
    shift
    [[ ${1:-} == -f ]]
    shift
    printf '%s\n' "$*" >> "$FAKE_DOCKER_CALLS"
    if [[ ${FAKE_DOCKER_KEEP:-0} != 1 ]]; then
      touch "$FAKE_DOCKER_STATE_DIR/${MIDAZ_SHARD_JOB_INDEX:-wave}.removed"
    fi
    ;;
  *)
    echo "unexpected docker command: $*" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$test_dir/bin/docker"

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
  INTEGRATION_RACE=1 \
  INTEGRATION_SHUFFLE_SEED=417 \
  TEST_REPORTS_DIR="$test_dir/reports" \
  "$repo_root/scripts/run-integration-shard.sh" tracer

grep -q $'parallel\texample.test/tracer-worker\towner-tracer\t2\t600s\t.*-race.*-parallel 3.*-shuffle=417' "$test_dir/calls/001.tsv"
grep -q $'parallel\texample.test/tracer-cache\towner-tracer\t2\t600s\t.*-race.*-parallel 3.*-shuffle=417' "$test_dir/calls/002.tsv"
grep -q $'serial\texample.test/tracer-journey\towner-tracer\t1\t600s\t.*-race.*-parallel 1.*-shuffle=417' "$test_dir/calls/003.tsv"
grep -q '"selected_test_count":4' "$test_dir/reports/tracer/summary.json"
grep -q '"failed_jobs":0' "$test_dir/reports/tracer/summary.json"
grep -q '"race_enabled":true' "$test_dir/reports/tracer/summary.json"
test -s "$test_dir/reports/tracer/selection.sha256"
test -s "$test_dir/reports/tracer/jobs/003/events.json"
test -s "$test_dir/reports/tracer/jobs/003/junit.xml"

cat > "$test_dir/lifecycle-plan.tsv" <<'EOF'
lifecycle-migration	serial	example.test/lifecycle-one	TestLifecycleOne
lifecycle-migration	serial	example.test/lifecycle-two	TestLifecycleTwo
EOF
mkdir -p "$test_dir/docker-state"
: > "$test_dir/docker-calls"
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-calls" \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-state" \
  TESTCONTAINERS_SESSION_ID=owner-lifecycle \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/lifecycle-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=1 \
  INTEGRATION_TEST_PARALLELISM=1 \
  TEST_REPORTS_DIR="$test_dir/lifecycle-reports" \
  "$repo_root/scripts/run-integration-shard.sh" lifecycle-migration

if [[ $(grep -Fxc 'deadbeefdead' "$test_dir/docker-calls") -ne 2 ]]; then
  echo "lifecycle shard did not clean after both serial jobs" >&2
  exit 1
fi
if grep -Fq 'feedfacefeed' "$test_dir/docker-calls"; then
  echo "lifecycle job cleanup attempted to remove Ryuk" >&2
  exit 1
fi
grep -q $'container_id\tname\timage\toutcome' "$test_dir/lifecycle-reports/lifecycle-migration/jobs/001/container-cleanup.tsv"
grep -q $'\tremoved$' "$test_dir/lifecycle-reports/lifecycle-migration/jobs/001/container-cleanup.tsv"
grep -q '"serial_cleanup_failures":0' "$test_dir/lifecycle-reports/lifecycle-migration/summary.json"

mkdir -p "$test_dir/docker-failure-state"
: > "$test_dir/docker-failure-calls"
status=0
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-failure-calls" \
  FAKE_DOCKER_KEEP=1 \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-failure-state" \
  TESTCONTAINERS_SESSION_ID=owner-lifecycle-failure \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/lifecycle-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=1 \
  INTEGRATION_TEST_PARALLELISM=1 \
  TEST_REPORTS_DIR="$test_dir/lifecycle-failure-reports" \
  "$repo_root/scripts/run-integration-shard.sh" lifecycle-migration || status=$?
if [[ $status -eq 0 ]]; then
  echo "failed lifecycle job cleanup produced a passing shard" >&2
  exit 1
fi
grep -q '"serial_cleanup_failures":2' "$test_dir/lifecycle-failure-reports/lifecycle-migration/summary.json"
grep -q $'cleanup=1$' "$test_dir/lifecycle-failure-reports/lifecycle-migration/failures.tsv"
grep -q $'\tfailed$' "$test_dir/lifecycle-failure-reports/lifecycle-migration/jobs/001/container-cleanup.tsv"

cat > "$test_dir/async-plan.tsv" <<'EOF'
async-broker	parallel	example.test/async-one	TestAsyncOne
async-broker	parallel	example.test/async-two	TestAsyncTwo
async-broker	parallel	example.test/async-three	TestAsyncThree
async-broker	parallel	example.test/async-four	TestAsyncFour
async-broker	parallel	example.test/async-five	TestAsyncFive
EOF
mkdir -p "$test_dir/docker-async-state"
: > "$test_dir/docker-async-calls"
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-async-calls" \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-async-state" \
  FAKE_RESET_WAVE_CLEANUP=1 \
  TESTCONTAINERS_SESSION_ID=owner-async \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/async-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=2 \
  TEST_REPORTS_DIR="$test_dir/async-reports" \
  "$repo_root/scripts/run-integration-shard.sh" async-broker

if [[ $(grep -Fxc 'deadbeefdead' "$test_dir/docker-async-calls") -ne 2 ]]; then
  echo "async shard did not clean after both parallel waves" >&2
  exit 1
fi
if grep -Fq 'feedfacefeed' "$test_dir/docker-async-calls"; then
  echo "async wave cleanup attempted to remove Ryuk" >&2
  exit 1
fi
test -s "$test_dir/async-reports/async-broker/parallel-waves/001/container-cleanup.tsv"
test -s "$test_dir/async-reports/async-broker/parallel-waves/002/container-cleanup.tsv"
grep -q '"parallel_cleanup_failures":0' "$test_dir/async-reports/async-broker/summary.json"

mkdir -p "$test_dir/docker-async-failure-state"
: > "$test_dir/docker-async-failure-calls"
status=0
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-async-failure-calls" \
  FAKE_DOCKER_KEEP=1 \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-async-failure-state" \
  FAKE_RESET_WAVE_CLEANUP=1 \
  TESTCONTAINERS_SESSION_ID=owner-async-failure \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/async-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=2 \
  TEST_REPORTS_DIR="$test_dir/async-failure-reports" \
  "$repo_root/scripts/run-integration-shard.sh" async-broker || status=$?
if [[ $status -eq 0 ]]; then
  echo "failed async wave cleanup produced a passing shard" >&2
  exit 1
fi
grep -q '"parallel_cleanup_failures":2' "$test_dir/async-failure-reports/async-broker/summary.json"
grep -q '"failed_jobs":0' "$test_dir/async-failure-reports/async-broker/summary.json"
grep -q $'wave-001\tparallel\towner-containers\tcleanup=1' "$test_dir/async-failure-reports/async-broker/failures.tsv"
grep -q $'\tfailed$' "$test_dir/async-failure-reports/async-broker/parallel-waves/001/container-cleanup.tsv"

cat > "$test_dir/mongo-plan.tsv" <<'EOF'
ledger-mongodb-crm	parallel	example.test/mongo-one	TestMongoOne
ledger-mongodb-crm	parallel	example.test/mongo-two	TestMongoTwo
ledger-mongodb-crm	parallel	example.test/mongo-three	TestMongoThree
ledger-mongodb-crm	parallel	example.test/mongo-four	TestMongoFour
ledger-mongodb-crm	parallel	example.test/mongo-five	TestMongoFive
EOF
mkdir -p "$test_dir/docker-mongo-state"
: > "$test_dir/docker-mongo-calls"
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-mongo-calls" \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-mongo-state" \
  FAKE_RESET_WAVE_CLEANUP=1 \
  TESTCONTAINERS_SESSION_ID=owner-mongo \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/mongo-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=2 \
  TEST_REPORTS_DIR="$test_dir/mongo-reports" \
  "$repo_root/scripts/run-integration-shard.sh" ledger-mongodb-crm

if [[ $(grep -Fxc 'deadbeefdead' "$test_dir/docker-mongo-calls") -ne 2 ]]; then
  echo "MongoDB/CRM shard did not clean after both parallel waves" >&2
  exit 1
fi
if grep -Fq 'feedfacefeed' "$test_dir/docker-mongo-calls"; then
  echo "MongoDB/CRM wave cleanup attempted to remove Ryuk" >&2
  exit 1
fi
test -s "$test_dir/mongo-reports/ledger-mongodb-crm/parallel-waves/001/container-cleanup.tsv"
test -s "$test_dir/mongo-reports/ledger-mongodb-crm/parallel-waves/002/container-cleanup.tsv"
grep -q '"parallel_cleanup_failures":0' "$test_dir/mongo-reports/ledger-mongodb-crm/summary.json"

mkdir -p "$test_dir/docker-mongo-failure-state"
: > "$test_dir/docker-mongo-failure-calls"
status=0
PATH="$test_dir/bin:$PATH" \
  FAKE_CALLS_DIR="$test_dir/calls" \
  FAKE_DOCKER_CALLS="$test_dir/docker-mongo-failure-calls" \
  FAKE_DOCKER_KEEP=1 \
  FAKE_DOCKER_STATE_DIR="$test_dir/docker-mongo-failure-state" \
  FAKE_RESET_WAVE_CLEANUP=1 \
  TESTCONTAINERS_SESSION_ID=owner-mongo-failure \
  INTEGRATION_SHARD_PLAN_FILE="$test_dir/mongo-plan.tsv" \
  INTEGRATION_PACKAGE_PARALLELISM=2 \
  INTEGRATION_TEST_PARALLELISM=2 \
  TEST_REPORTS_DIR="$test_dir/mongo-failure-reports" \
  "$repo_root/scripts/run-integration-shard.sh" ledger-mongodb-crm || status=$?
if [[ $status -eq 0 ]]; then
  echo "failed MongoDB/CRM wave cleanup produced a passing shard" >&2
  exit 1
fi
grep -q '"parallel_cleanup_failures":2' "$test_dir/mongo-failure-reports/ledger-mongodb-crm/summary.json"
grep -q '"failed_jobs":0' "$test_dir/mongo-failure-reports/ledger-mongodb-crm/summary.json"
grep -q $'wave-001\tparallel\towner-containers\tcleanup=1' "$test_dir/mongo-failure-reports/ledger-mongodb-crm/failures.tsv"
grep -q $'\tfailed$' "$test_dir/mongo-failure-reports/ledger-mongodb-crm/parallel-waves/001/container-cleanup.tsv"

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
