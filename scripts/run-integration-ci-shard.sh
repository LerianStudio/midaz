#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <integration-shard>" >&2
  exit 2
fi

shard=$1
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
config=${INTEGRATION_SHARD_CONFIG:-$repo_root/ci/integration-shards.tsv}

config_row=$(awk -F '\t' -v selected="$shard" '
  !/^#/ && NF && $1 == selected { print; found++ }
  END {
    if (found != 1) {
      printf "shard %s has %d config rows, want exactly one\n", selected, found > "/dev/stderr"
      exit 2
    }
  }
' "$config")
IFS=$'\t' read -r _ package_parallelism test_parallelism job_gomaxprocs \
  max_average_cpu_percent max_rss_mb max_peak_containers flake_budget wall_timeout \
  race_enabled <<< "$config_row"

run_id=${GITHUB_RUN_ID:-local-$$-$(date +%s)}
run_attempt=${GITHUB_RUN_ATTEMPT:-0}
docker_owner="midaz-$run_id-$run_attempt-$shard"
shuffle_seed=${INTEGRATION_SHUFFLE_SEED:-${GITHUB_RUN_ID:-on}}
lane_runner=${MIDAZ_CI_LANE_RUNNER:-$repo_root/scripts/run-ci-lane.sh}
ci_report_dir=${CI_REPORT_DIR:-reports/ci}
test_reports_dir=${TEST_REPORTS_DIR:-reports/ci/integration-shards}
lane_command=(make test-integration-shard "INTEGRATION_SHARD=$shard")

# Blacksmith supplies exactly four CPUs. Mordor is intentionally much larger,
# so give concurrent local shard benchmarks disjoint four-CPU scopes instead
# of letting compiler subprocesses turn a CI budget into a 256-core fantasy.
if [[ ${INTEGRATION_CPUSET:-auto} == auto && $(nproc) -gt 4 && -n $(command -v taskset || true) ]]; then
  case $shard in
    ledger-postgres) cpu_start=0 ;;
    ledger-mongodb-crm) cpu_start=4 ;;
    async-broker) cpu_start=8 ;;
    tracer) cpu_start=12 ;;
    lifecycle-migration) cpu_start=16 ;;
    chaos-capability) cpu_start=20 ;;
  esac
  cpu_end=$((cpu_start + 3))
  lane_command=(taskset --cpu-list "$cpu_start-$cpu_end" "${lane_command[@]}")
elif [[ ${INTEGRATION_CPUSET:-auto} != auto && ${INTEGRATION_CPUSET:-auto} != off ]]; then
  echo "INTEGRATION_CPUSET must be auto or off" >&2
  exit 2
fi

cd "$repo_root"
exec env \
  CI_CAPTURE_DOCKER_EVENTS=owner \
  CI_REQUIRE_DOCKER_OWNER_EVENTS=1 \
  CI_DOCKER_OWNER="$docker_owner" \
  CI_CAPTURE_RESOURCES=1 \
  CI_MAX_AVERAGE_CPU_PERCENT="$max_average_cpu_percent" \
  CI_MAX_RSS_MB="$max_rss_mb" \
  CI_MAX_PEAK_CONTAINERS="$max_peak_containers" \
  CI_REPORT_DIR="$ci_report_dir" \
  TEST_REPORTS_DIR="$test_reports_dir" \
  INTEGRATION_PACKAGE_PARALLELISM="$package_parallelism" \
  INTEGRATION_TEST_PARALLELISM="$test_parallelism" \
  INTEGRATION_JOB_GOMAXPROCS="$job_gomaxprocs" \
  GOMAXPROCS="$job_gomaxprocs" \
  INTEGRATION_FLAKE_BUDGET="$flake_budget" \
  INTEGRATION_RACE="$race_enabled" \
  INTEGRATION_SHUFFLE_SEED="$shuffle_seed" \
  "$lane_runner" "integration-$shard" "$wall_timeout" \
    "${lane_command[@]}"
