#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

runner_started_epoch=$(date +%s)

usage() {
  echo "usage: $0 <ledger-postgres|ledger-mongodb-crm|async-broker|tracer|lifecycle-migration>" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

shard=$1
case $shard in
  ledger-postgres|ledger-mongodb-crm|async-broker|tracer|lifecycle-migration) ;;
  *)
    usage
    exit 2
    ;;
esac

package_parallelism=${INTEGRATION_PACKAGE_PARALLELISM:-2}
test_parallelism=${INTEGRATION_TEST_PARALLELISM:-2}
shuffle_seed=${INTEGRATION_SHUFFLE_SEED:-on}
flake_budget=${INTEGRATION_FLAKE_BUDGET:-0}
test_timeout=${INTEGRATION_TEST_TIMEOUT:-600s}
race_enabled=${INTEGRATION_RACE:-0}
job_gomaxprocs=${INTEGRATION_JOB_GOMAXPROCS:-2}
ryuk_reconnection_timeout=${RYUK_RECONNECTION_TIMEOUT:-$test_timeout}

validate_parallelism() {
  local label=$1
  local value=$2
  if [[ ! $value =~ ^[1-4]$ ]]; then
    echo "$label must be an integer from 1 through 4, got '$value'" >&2
    exit 2
  fi
}

validate_parallelism INTEGRATION_PACKAGE_PARALLELISM "$package_parallelism"
validate_parallelism INTEGRATION_TEST_PARALLELISM "$test_parallelism"
validate_parallelism INTEGRATION_JOB_GOMAXPROCS "$job_gomaxprocs"
if [[ $flake_budget != 0 ]]; then
  echo "INTEGRATION_FLAKE_BUDGET must remain 0; shard retries are intentionally disabled" >&2
  exit 2
fi
if [[ $race_enabled != 0 && $race_enabled != 1 ]]; then
  echo "INTEGRATION_RACE must be 0 or 1, got '$race_enabled'" >&2
  exit 2
fi
if [[ ! $test_timeout =~ ^[0-9]+(ms|s|m|h)$ ]]; then
  echo "INTEGRATION_TEST_TIMEOUT must be a Go duration, got '$test_timeout'" >&2
  exit 2
fi
if [[ ! $ryuk_reconnection_timeout =~ ^[0-9]+(ms|s|m|h)$ ]]; then
  echo "RYUK_RECONNECTION_TIMEOUT must be a Go duration, got '$ryuk_reconnection_timeout'" >&2
  exit 2
fi
if [[ $shuffle_seed != on && $shuffle_seed != off && ! $shuffle_seed =~ ^[0-9]+$ ]]; then
  echo "INTEGRATION_SHUFFLE_SEED must be on, off, or a non-negative integer" >&2
  exit 2
fi
export GOMAXPROCS=$job_gomaxprocs

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
skip_allowlist=${INTEGRATION_SKIP_ALLOWLIST:-$repo_root/ci/integration-skip-allowlist.json}
if [[ $skip_allowlist != /* ]]; then
  skip_allowlist=$repo_root/$skip_allowlist
fi
if [[ ! -r $skip_allowlist ]]; then
  echo "integration skip allowlist is not readable: $skip_allowlist" >&2
  exit 2
fi
report_root=${TEST_REPORTS_DIR:-$repo_root/reports/integration-shards}
if [[ $report_root != /* ]]; then
  report_root=$repo_root/$report_root
fi
report_dir=$report_root/$shard
jobs_dir=$report_dir/jobs
mkdir -p "$jobs_dir"

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/midaz-integration-shard.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT
shard_tool=$work_dir/integration-shards
(cd "$repo_root" && go build -o "$shard_tool" ./scripts/integration_shards)
cd "$repo_root"

full_plan=$work_dir/full-plan.tsv
if [[ -n ${INTEGRATION_SHARD_PLAN_FILE:-} ]]; then
  cp "$INTEGRATION_SHARD_PLAN_FILE" "$full_plan"
else
  inventory=$report_dir/inventory.tsv
  (cd "$repo_root" && \
    scripts/list-tagged-test-functions.sh integration integration \
      ./components/... ./pkg/... ./tests/...) > "$inventory"
  "$shard_tool" --skip-allowlist "$skip_allowlist" < "$inventory" > "$full_plan"
fi

selection=$report_dir/selection.tsv
awk -F '\t' -v selected_shard="$shard" '
  NF != 4 { printf "invalid shard plan line %d: want 4 tab-separated fields\n", NR > "/dev/stderr"; exit 2 }
  $1 == selected_shard { print $2 "\t" $3 "\t" $4 }
' "$full_plan" > "$selection"
if [[ ! -s $selection ]]; then
  echo "shard '$shard' selected zero integration tests" >&2
  exit 2
fi
if awk -F '\t' '$1 != "parallel" && $1 != "serial" { exit 1 }' "$selection"; then
  :
else
  echo "shard '$shard' contains an unknown execution mode" >&2
  exit 2
fi
if duplicate=$(sort "$selection" | uniq -d | head -n 1) && [[ -n $duplicate ]]; then
  echo "shard '$shard' contains a duplicate test assignment: $duplicate" >&2
  exit 2
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$selection" > "$report_dir/selection.sha256"
else
  shasum -a 256 "$selection" > "$report_dir/selection.sha256"
fi

jobs_manifest=$report_dir/jobs.tsv
: > "$jobs_manifest"
job_index=0
previous_key=
declare -A seen_job_keys=()
while IFS=$'\t' read -r mode package test_name; do
  key="$mode|$package"
  if [[ $key != "$previous_key" ]]; then
    if [[ -n ${seen_job_keys[$key]:-} ]]; then
      echo "non-contiguous package assignment in shard plan: $mode $package" >&2
      exit 2
    fi
    seen_job_keys[$key]=1
    job_index=$((job_index + 1))
    printf -v padded_index '%03d' "$job_index"
    job_dir="$jobs_dir/$padded_index"
    mkdir -p "$job_dir"
    : > "$job_dir/selected-tests.txt"
    printf '%s\t%s\t%s\t%s\n' "$padded_index" "$mode" "$package" "$job_dir" >> "$jobs_manifest"
    previous_key=$key
  fi
  printf '%s\n' "$test_name" >> "$job_dir/selected-tests.txt"
done < "$selection"

if [[ -z ${TESTCONTAINERS_SESSION_ID:-} ]]; then
  TESTCONTAINERS_SESSION_ID="midaz-$shard-$$-$(date +%s)"
  export TESTCONTAINERS_SESSION_ID
fi

export ALLOW_INSECURE_TLS=true
export CHAOS=${CHAOS:-0}
export GOFLAGS="-buildvcs=false ${GOFLAGS:-}"
export INTEGRATION_PACKAGE_PARALLELISM="$package_parallelism"
export INTEGRATION_TEST_PARALLELISM="$test_parallelism"
export INTEGRATION_SHUFFLE_SEED="$shuffle_seed"
export INTEGRATION_TEST_TIMEOUT="$test_timeout"
export INTEGRATION_RACE="$race_enabled"
export INTEGRATION_JOB_GOMAXPROCS="$job_gomaxprocs"
export MIDAZ_INTEGRATION_SHARD_TOOL="$shard_tool"
export RYUK_RECONNECTION_TIMEOUT="$ryuk_reconnection_timeout"

serial_cleanup_failures=0
parallel_cleanup_failures=0
docker_bin=
cleanup_timeout_bin=
owner_cleanup_timeout=${INTEGRATION_OWNER_CLEANUP_TIMEOUT_SECONDS:-30}
if [[ $shard == lifecycle-migration || $shard == async-broker || $shard == ledger-mongodb-crm ]]; then
  docker_bin=$(command -v docker || true)
  cleanup_timeout_bin=$(command -v timeout || command -v gtimeout || true)
  if [[ -z $docker_bin || -z $cleanup_timeout_bin ]]; then
    echo "$shard requires Docker and GNU timeout for bounded owner cleanup" >&2
    exit 2
  fi
  if [[ ! $owner_cleanup_timeout =~ ^[1-9][0-9]*$ ]]; then
    echo "INTEGRATION_OWNER_CLEANUP_TIMEOUT_SECONDS must be a positive integer" >&2
    exit 2
  fi
fi
export shard skip_allowlist docker_bin cleanup_timeout_bin owner_cleanup_timeout

cleanup_shard_owner_containers() {
  local job_dir=$1
  local cleanup_file="$job_dir/container-cleanup.tsv"
  local cleanup_log="$job_dir/container-cleanup.log"
  local inventory_file="$job_dir/container-cleanup-inventory.tmp"
  local current_inventory current_ids container_id name image labels identity index outcome
  local cleanup_failures=0
  local -a cleanup_ids=() cleanup_names=() cleanup_images=()

  printf 'container_id\tname\timage\toutcome\n' > "$cleanup_file"
  : > "$cleanup_log"
  if ! "$docker_bin" ps -a --no-trunc \
    --filter "label=org.testcontainers.sessionId=$TESTCONTAINERS_SESSION_ID" \
    --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Labels}}' > "$inventory_file"; then
    echo "could not inspect shard owner containers before cleanup" >> "$cleanup_log"
    rm -f "$inventory_file"
    return 1
  fi

  while IFS=$'\t' read -r container_id name image labels; do
    [[ -n $container_id ]] || continue
    identity=${name,,}' '${image,,}' '${labels,,}
    if [[ $identity == *testcontainers/ryuk* || $identity =~ (^|[[:space:]])reaper[_-] || \
      $identity == *org.testcontainers.ryuk=true* ]]; then
      continue
    fi
    if [[ ! $container_id =~ ^[a-f0-9]{12,64}$ ]]; then
      printf '%s\t%s\t%s\tinvalid-id\n' "$container_id" "$name" "$image" >> "$cleanup_file"
      cleanup_failures=$((cleanup_failures + 1))
      continue
    fi
    cleanup_ids+=("$container_id")
    cleanup_names+=("$name")
    cleanup_images+=("$image")
  done < "$inventory_file"
  rm -f "$inventory_file"

  if [[ ${#cleanup_ids[@]} -gt 0 ]]; then
    "$cleanup_timeout_bin" --foreground --kill-after=5s "${owner_cleanup_timeout}s" \
      "$docker_bin" rm -f "${cleanup_ids[@]}" >> "$cleanup_log" 2>&1 || true
  fi

  if ! current_inventory=$("$docker_bin" ps -a --no-trunc \
    --filter "label=org.testcontainers.sessionId=$TESTCONTAINERS_SESSION_ID" \
    --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Labels}}'); then
    echo "could not inspect shard owner containers after cleanup" >> "$cleanup_log"
    for index in "${!cleanup_ids[@]}"; do
      printf '%s\t%s\t%s\tunknown\n' \
        "${cleanup_ids[$index]}" "${cleanup_names[$index]}" "${cleanup_images[$index]}" \
        >> "$cleanup_file"
      cleanup_failures=$((cleanup_failures + 1))
    done
    return 1
  fi
  current_ids=$(awk -F '\t' 'NF { print $1 }' <<< "$current_inventory")

  for index in "${!cleanup_ids[@]}"; do
    outcome=removed
    if grep -Fxq "${cleanup_ids[$index]}" <<< "$current_ids"; then
      outcome=failed
      cleanup_failures=$((cleanup_failures + 1))
    fi
    printf '%s\t%s\t%s\t%s\n' \
      "${cleanup_ids[$index]}" "${cleanup_names[$index]}" "${cleanup_images[$index]}" "$outcome" \
      >> "$cleanup_file"
  done

  [[ $cleanup_failures -eq 0 ]]
}
export -f cleanup_shard_owner_containers

run_shard_job() {
  local index=$1
  local mode=$2
  local package=$3
  local job_dir=$4
  local selected_tests=$job_dir/selected-tests.txt
  local events=$job_dir/events.json
  local junit=$job_dir/junit.xml
  local log=$job_dir/test.log
  local outcomes=$job_dir/outcomes.tsv
  local status_file=$job_dir/status.tsv
  local effective_test_parallelism=$INTEGRATION_TEST_PARALLELISM
  local effective_package_parallelism=$INTEGRATION_PACKAGE_PARALLELISM
  if [[ $mode == serial ]]; then
    effective_test_parallelism=1
    effective_package_parallelism=1
  fi

  local alternation
  alternation=$(paste -sd'|' "$selected_tests")
  local exact_pattern="^(${alternation})\$"
  local -a go_args=(
    -tags=integration
    -json
    -v
    -count=1
    "-timeout=$INTEGRATION_TEST_TIMEOUT"
    -p 1
    -parallel "$effective_test_parallelism"
    "-shuffle=$INTEGRATION_SHUFFLE_SEED"
    -run "$exact_pattern"
    "$package"
  )
  if [[ $INTEGRATION_RACE == 1 ]]; then
    go_args=(-race "${go_args[@]}")
  fi

  export MIDAZ_SHARD_JOB_INDEX=$index
  export MIDAZ_SHARD_JOB_MODE=$mode
  export MIDAZ_SHARD_JOB_PACKAGE=$package
  export MIDAZ_SHARD_JOB_SELECTION_FILE=$selected_tests
  export INTEGRATION_PACKAGE_PARALLELISM=$effective_package_parallelism
  export GOMAXPROCS=$INTEGRATION_JOB_GOMAXPROCS

  local command_status=0
  if command -v gotestsum >/dev/null 2>&1; then
    local -a reporter_args=()
    for arg in "${go_args[@]}"; do
      if [[ $arg != -json ]]; then
        reporter_args+=("$arg")
      fi
    done
    set +e
    gotestsum "--jsonfile=$events" "--junitfile=$junit" --format testname -- \
      "${reporter_args[@]}" 2>&1 | tee "$log"
    command_status=${PIPESTATUS[0]}
    set -e
  else
    set +e
    go test "${go_args[@]}" > "$events" 2> >(tee "$log" >&2)
    command_status=$?
    set -e
  fi

  local verifier_status=0
  if ! "$MIDAZ_INTEGRATION_SHARD_TOOL" verify-events \
    --package "$package" --expected "$selected_tests" --events "$events" \
    --skip-allowlist "$skip_allowlist" --outcomes "$outcomes" \
    >> "$log" 2>&1; then
    verifier_status=1
  fi
  local cleanup_status=0
  if [[ $shard == lifecycle-migration ]] && ! cleanup_shard_owner_containers "$job_dir"; then
    cleanup_status=1
  fi
  printf '%s\t%s\t%s\t%d\t%d\t%d\n' \
    "$index" "$mode" "$package" "$command_status" "$verifier_status" "$cleanup_status" \
    > "$status_file"

  # A job failure is recorded and aggregated after every peer has finished.
  # Returning success here prevents xargs from suppressing later attribution.
  return 0
}
export -f run_shard_job

parallel_jobs=$work_dir/parallel-jobs.tsv
serial_jobs=$work_dir/serial-jobs.tsv
parallel_cleanup_failures_file=$work_dir/parallel-cleanup-failures.tsv
awk -F '\t' '$2 == "parallel"' "$jobs_manifest" > "$parallel_jobs"
awk -F '\t' '$2 == "serial"' "$jobs_manifest" > "$serial_jobs"
: > "$parallel_cleanup_failures_file"

run_bounded_parallel_wave() {
  local wave_manifest=$1
  local wave_index=$2
  local wave_dir="$report_dir/parallel-waves/$wave_index"

  xargs -P "$package_parallelism" -n 4 bash -c 'run_shard_job "$@"' _ < "$wave_manifest"
  mkdir -p "$wave_dir"
  if ! cleanup_shard_owner_containers "$wave_dir"; then
    parallel_cleanup_failures=$((parallel_cleanup_failures + 1))
    printf 'wave-%s\tparallel\towner-containers\tcleanup=1\n' "$wave_index" \
      >> "$parallel_cleanup_failures_file"
  fi
}

echo "[$shard] selected $(wc -l < "$selection" | tr -d ' ') tests across $job_index package runs"
echo "[$shard] package parallelism=$package_parallelism, in-package parallelism=$test_parallelism, shuffle=$shuffle_seed, flake budget=$flake_budget"

if [[ -s $parallel_jobs ]]; then
  if [[ $shard == async-broker || $shard == ledger-mongodb-crm ]]; then
    parallel_wave_size=$((package_parallelism * 2))
    wave_index=0
    wave_jobs=0
    wave_manifest=
    while IFS=$'\t' read -r index mode package job_dir; do
      [[ -n $index ]] || continue
      if [[ $wave_jobs -eq 0 ]]; then
        wave_index=$((wave_index + 1))
        printf -v padded_wave '%03d' "$wave_index"
        wave_manifest="$work_dir/parallel-wave-$padded_wave.tsv"
        : > "$wave_manifest"
      fi
      printf '%s\t%s\t%s\t%s\n' "$index" "$mode" "$package" "$job_dir" >> "$wave_manifest"
      wave_jobs=$((wave_jobs + 1))
      if [[ $wave_jobs -eq $parallel_wave_size ]]; then
        run_bounded_parallel_wave "$wave_manifest" "$padded_wave"
        wave_jobs=0
      fi
    done < "$parallel_jobs"
    if [[ $wave_jobs -gt 0 ]]; then
      run_bounded_parallel_wave "$wave_manifest" "$padded_wave"
    fi
  else
    xargs -P "$package_parallelism" -n 4 bash -c 'run_shard_job "$@"' _ < "$parallel_jobs"
  fi
fi
while IFS=$'\t' read -r index mode package job_dir; do
  [[ -n $index ]] || continue
  run_shard_job "$index" "$mode" "$package" "$job_dir"
done < "$serial_jobs"

failures=$report_dir/failures.tsv
: > "$failures"
cat "$parallel_cleanup_failures_file" >> "$failures"
failed_jobs=0
while IFS=$'\t' read -r index mode package job_dir; do
  status_file=$job_dir/status.tsv
  if [[ ! -s $status_file ]]; then
    printf '%s\t%s\t%s\tobserver-missing\n' "$index" "$mode" "$package" >> "$failures"
    failed_jobs=$((failed_jobs + 1))
    continue
  fi
  IFS=$'\t' read -r _ _ _ command_status verifier_status cleanup_status < "$status_file"
  if [[ $command_status -ne 0 || $verifier_status -ne 0 || $cleanup_status -ne 0 ]]; then
    printf '%s\t%s\t%s\tcommand=%s\tverification=%s\tcleanup=%s\n' \
      "$index" "$mode" "$package" "$command_status" "$verifier_status" "$cleanup_status" >> "$failures"
    failed_jobs=$((failed_jobs + 1))
    if [[ $cleanup_status -ne 0 ]]; then
      serial_cleanup_failures=$((serial_cleanup_failures + 1))
    fi
  fi
done < "$jobs_manifest"

selected_test_count=$(wc -l < "$selection" | tr -d ' ')
parallel_test_count=$(awk -F '\t' '$1 == "parallel" { count++ } END { print count + 0 }' "$selection")
serial_test_count=$(awk -F '\t' '$1 == "serial" { count++ } END { print count + 0 }' "$selection")
outcomes=$report_dir/outcomes.tsv
printf 'package\ttest\toutcome\treason\talternate_capability\n' > "$outcomes"
while IFS=$'\t' read -r _ _ _ job_dir; do
  [[ -s $job_dir/outcomes.tsv ]] || continue
  tail -n +2 "$job_dir/outcomes.tsv" >> "$outcomes"
done < "$jobs_manifest"
passed_test_count=$(awk -F '\t' '$3 == "passed" { count++ } END { print count + 0 }' "$outcomes")
skipped_test_count=$(awk -F '\t' '$3 == "skipped" { count++ } END { print count + 0 }' "$outcomes")
failed_test_count=$(awk -F '\t' '$3 == "failed" { count++ } END { print count + 0 }' "$outcomes")
missing_test_count=$(awk -F '\t' '$3 == "missing" { count++ } END { print count + 0 }' "$outcomes")
classified_test_count=$((passed_test_count + skipped_test_count + failed_test_count + missing_test_count))
unclassified_test_count=$((selected_test_count - classified_test_count))
classification_integrity_failures=0
unknown_outcome_count=$(awk -F '\t' 'NR > 1 && $3 != "passed" && $3 != "skipped" && $3 != "failed" && $3 != "missing" { count++ } END { print count + 0 }' "$outcomes")
if [[ $unclassified_test_count -ne 0 || $unknown_outcome_count -ne 0 ]]; then
  classification_integrity_failures=1
  printf 'outcomes\tclassification\tunion\tselected=%d\tclassified=%d\n' \
    "$selected_test_count" "$classified_test_count" >> "$failures"
  if [[ $unclassified_test_count -lt 0 ]]; then
    unclassified_test_count=0
  fi
fi
covered_test_count=$((passed_test_count + skipped_test_count))
uncovered_test_count=$((failed_test_count + missing_test_count + unclassified_test_count))
if ((covered_test_count + uncovered_test_count != selected_test_count)); then
  classification_integrity_failures=1
fi
runner_duration_seconds=$(($(date +%s) - runner_started_epoch))
race_enabled_json=false
if [[ $race_enabled == 1 ]]; then
  race_enabled_json=true
fi
printf '{"shard":"%s","selected_test_count":%d,"covered_test_count":%d,"passed_test_count":%d,"skipped_test_count":%d,"failed_test_count":%d,"missing_test_count":%d,"unclassified_test_count":%d,"uncovered_test_count":%d,"parallel_test_count":%d,"serial_test_count":%d,"package_runs":%d,"package_parallelism":%d,"test_parallelism":%d,"job_gomaxprocs":%d,"shuffle_seed":"%s","flake_budget":%d,"race_enabled":%s,"serial_cleanup_failures":%d,"parallel_cleanup_failures":%d,"classification_integrity_failures":%d,"failed_jobs":%d,"duration_seconds":%d}\n' \
  "$shard" "$selected_test_count" "$covered_test_count" "$passed_test_count" "$skipped_test_count" "$failed_test_count" "$missing_test_count" "$unclassified_test_count" "$uncovered_test_count" "$parallel_test_count" "$serial_test_count" "$job_index" \
  "$package_parallelism" "$test_parallelism" "$job_gomaxprocs" "$shuffle_seed" "$flake_budget" "$race_enabled_json" "$serial_cleanup_failures" "$parallel_cleanup_failures" "$classification_integrity_failures" "$failed_jobs" "$runner_duration_seconds" \
  > "$report_dir/summary.json"

if [[ $failed_jobs -ne 0 || $parallel_cleanup_failures -ne 0 || $classification_integrity_failures -ne 0 || $uncovered_test_count -ne 0 ]]; then
  echo "[$shard] failed: $failed_jobs package run(s), $parallel_cleanup_failures parallel cleanup failure(s), $uncovered_test_count uncovered test(s); see $failures" >&2
  exit 1
fi

echo "[$shard] covered: $passed_test_count passed, $skipped_test_count allowlisted skipped, zero retries"
