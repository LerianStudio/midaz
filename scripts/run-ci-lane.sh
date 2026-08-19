#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: $0 <lane> <wall-timeout> <command> [args...]" >&2
  exit 2
fi

lane=$1
wall_timeout=$2
shift 2

if [[ ! $lane =~ ^[a-zA-Z0-9._-]+$ ]]; then
  echo "invalid lane name: $lane" >&2
  exit 2
fi
if [[ ! $wall_timeout =~ ^[0-9]+(ms|s|m|h)$ ]]; then
  echo "invalid wall timeout: $wall_timeout" >&2
  exit 2
fi

timeout_bin=$(command -v timeout || command -v gtimeout || true)
if [[ -z $timeout_bin ]]; then
  echo "required CI lane '$lane' needs GNU timeout (install coreutils on macOS)" >&2
  exit 2
fi

report_dir=${CI_REPORT_DIR:-reports/ci}
mkdir -p "$report_dir"

log_file="$report_dir/$lane.log"
timing_file="$report_dir/$lane-timing.json"
docker_events_file="$report_dir/$lane-docker-events.jsonl"
docker_summary_file="$report_dir/$lane-docker-summary.json"
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
started_epoch=$(date +%s)

docker_events_pid=
docker_events_failed=0
docker_owner=
stop_docker_events() {
  if [[ -n $docker_events_pid ]]; then
    if kill -0 "$docker_events_pid" 2>/dev/null; then
      kill "$docker_events_pid" 2>/dev/null || true
      wait "$docker_events_pid" 2>/dev/null || true
    else
      wait "$docker_events_pid" 2>/dev/null || true
      docker_events_failed=1
    fi
    docker_events_pid=
  fi
}

docker_event_scope=${CI_CAPTURE_DOCKER_EVENTS:-}
if [[ -n $docker_event_scope ]]; then
  docker_bin=$(command -v docker || true)
  if [[ -z $docker_bin ]]; then
    echo "required CI lane '$lane' cannot capture Docker events because docker is unavailable" >&2
    exit 2
  fi

  docker_event_filters=(--filter type=container)
  case $docker_event_scope in
    all) ;;
    testcontainers) docker_event_filters+=(--filter label=org.testcontainers=true) ;;
    owner)
      docker_owner=${CI_DOCKER_OWNER:-midaz-$lane-$$-$started_epoch}
      if [[ ! $docker_owner =~ ^[a-zA-Z0-9._-]+$ ]]; then
        echo "invalid CI_DOCKER_OWNER value: $docker_owner" >&2
        exit 2
      fi
      export TESTCONTAINERS_SESSION_ID=$docker_owner
      docker_event_filters+=(--filter "label=org.testcontainers.sessionId=$docker_owner")
      ;;
    *)
      echo "invalid CI_CAPTURE_DOCKER_EVENTS value: $docker_event_scope" >&2
      exit 2
      ;;
  esac

  "$timeout_bin" --kill-after=5s "$wall_timeout" \
    "$docker_bin" events "${docker_event_filters[@]}" --format '{{json .}}' \
    > "$docker_events_file" 2> "$report_dir/$lane-docker-events.log" &
  docker_events_pid=$!
  trap stop_docker_events EXIT

  # Docker can accept the process launch and fail before the lane starts. Do
  # not publish a zero-container measurement when the observer never lived.
  sleep 1
  if ! kill -0 "$docker_events_pid" 2>/dev/null; then
    wait "$docker_events_pid" 2>/dev/null || true
    docker_events_pid=
    trap - EXIT
    echo "required CI lane '$lane' Docker event observer exited before the lane started" >&2
    exit 2
  fi
fi

echo "[$lane] starting (wall timeout: $wall_timeout)"
capture_resources=${CI_CAPTURE_RESOURCES:-0}
if [[ $capture_resources != 0 && $capture_resources != 1 ]]; then
  echo "CI_CAPTURE_RESOURCES must be 0 or 1" >&2
  exit 2
fi
for budget_name in CI_MAX_RSS_MB CI_MAX_AVERAGE_CPU_PERCENT CI_MAX_PEAK_CONTAINERS; do
  budget_value=${!budget_name:-}
  if [[ -n $budget_value && ! $budget_value =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "$budget_name must be a non-negative number" >&2
    exit 2
  fi
done

cpu_time_file="$report_dir/$lane-cpu-time.txt"
lane_started_ns=$(date +%s%N)
set +e
{
  TIMEFORMAT='MIDAZ_CPU_TIME %U %S'
  time "$timeout_bin" --kill-after=30s "$wall_timeout" "$@" \
    > >(tee "$log_file") 2>&1
} 2> "$cpu_time_file" &
lane_pid=$!
set -e

resource_observer_pid=
resource_observer_failed=0
resource_raw_file="$report_dir/$lane-resources-raw.json"
resource_summary_file="$report_dir/$lane-resources.json"
resource_samples_file="$report_dir/$lane-resource-samples.tsv"
if [[ $capture_resources == 1 ]]; then
  resource_observer=${CI_RESOURCE_OBSERVER:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/observe-ci-process.sh}
  if [[ ! -x $resource_observer ]]; then
    echo "required CI lane '$lane' resource observer is not executable: $resource_observer" >&2
    kill "$lane_pid" 2>/dev/null || true
    wait "$lane_pid" 2>/dev/null || true
    exit 2
  fi
  "$resource_observer" "$lane_pid" "$resource_samples_file" "$resource_raw_file" &
  resource_observer_pid=$!
fi

set +e
wait "$lane_pid"
exit_code=$?
set -e
lane_finished_ns=$(date +%s%N)

if [[ -n $resource_observer_pid ]]; then
  if ! wait "$resource_observer_pid"; then
    resource_observer_failed=1
  fi
  if [[ $resource_observer_failed -eq 1 || ! -s $resource_raw_file ]]; then
    echo "required CI lane '$lane' resource observer failed before publishing a summary" >&2
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  else
    peak_rss_mb=$(sed -n 's/.*"peak_rss_mb":\([0-9]*\).*/\1/p' "$resource_raw_file")
    resource_samples=$(sed -n 's/.*"samples":\([0-9]*\).*/\1/p' "$resource_raw_file")
    read -r cpu_marker user_cpu_seconds system_cpu_seconds < "$cpu_time_file" || true
    if [[ $cpu_marker != MIDAZ_CPU_TIME || ! $user_cpu_seconds =~ ^[0-9]+([.][0-9]+)?$ || \
      ! $system_cpu_seconds =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      echo "required CI lane '$lane' could not read aggregate CPU time" >&2
      if [[ $exit_code -eq 0 ]]; then
        exit_code=2
      fi
    elif [[ -z $peak_rss_mb || -z $resource_samples ]]; then
      echo "required CI lane '$lane' resource observer published an invalid summary" >&2
      if [[ $exit_code -eq 0 ]]; then
        exit_code=2
      fi
    else
      read -r cpu_seconds average_cpu_percent < <(
        awk -v user="$user_cpu_seconds" -v syscpu="$system_cpu_seconds" \
          -v started="$lane_started_ns" -v finished="$lane_finished_ns" '
          BEGIN {
            cpu = user + syscpu
            wall = (finished - started) / 1000000000
            average = wall > 0 ? cpu * 100 / wall : 0
            printf "%.3f %.2f\n", cpu, average
          }
        '
      )
      budget_status=within
      if [[ -n ${CI_MAX_AVERAGE_CPU_PERCENT:-} ]] && \
        awk -v value="$average_cpu_percent" -v limit="$CI_MAX_AVERAGE_CPU_PERCENT" 'BEGIN { exit !(value > limit) }'; then
        budget_status=exceeded
      fi
      if [[ -n ${CI_MAX_RSS_MB:-} ]] && \
        awk -v value="$peak_rss_mb" -v limit="$CI_MAX_RSS_MB" 'BEGIN { exit !(value > limit) }'; then
        budget_status=exceeded
      fi
      printf '{"lane":"%s","samples":%d,"cpu_seconds":%s,"average_cpu_percent":%s,"peak_rss_mb":%d,"max_average_cpu_percent":"%s","max_rss_mb":"%s","budget_status":"%s"}\n' \
        "$lane" "$resource_samples" "$cpu_seconds" "$average_cpu_percent" "$peak_rss_mb" \
        "${CI_MAX_AVERAGE_CPU_PERCENT:-}" "${CI_MAX_RSS_MB:-}" "$budget_status" > "$resource_summary_file"
      if [[ $budget_status == exceeded && $exit_code -eq 0 ]]; then
        echo "required CI lane '$lane' exceeded its CPU or RSS budget" >&2
        exit_code=2
      fi
    fi
  fi
fi

stop_docker_events
trap - EXIT

if [[ $docker_events_failed -eq 1 ]]; then
  echo "required CI lane '$lane' Docker event observer exited before the lane finished" >&2
  if [[ $exit_code -eq 0 ]]; then
    exit_code=2
  fi
fi

if [[ -n $docker_event_scope && $docker_events_failed -eq 0 ]]; then
  read -r container_starts container_restarts peak_live_containers < <(
    awk '
      index($0, "\"Action\":\"start\"") { starts++; live++; if (live > peak) peak = live }
      index($0, "\"Action\":\"restart\"") { restarts++ }
      index($0, "\"Action\":\"die\"") { if (live > 0) live-- }
      END { printf "%d %d %d\n", starts + 0, restarts + 0, peak + 0 }
    ' "$docker_events_file"
  )
  container_budget_status=within
  if [[ -n ${CI_MAX_PEAK_CONTAINERS:-} ]] && \
    awk -v value="$peak_live_containers" -v limit="$CI_MAX_PEAK_CONTAINERS" 'BEGIN { exit !(value > limit) }'; then
    container_budget_status=exceeded
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  fi
  printf '{"lane":"%s","scope":"%s","owner":"%s","container_start_events":%d,"container_restart_events":%d,"peak_live_containers":%d,"max_peak_containers":"%s","budget_status":"%s"}\n' \
    "$lane" "$docker_event_scope" "$docker_owner" "$container_starts" "$container_restarts" \
    "$peak_live_containers" "${CI_MAX_PEAK_CONTAINERS:-}" "$container_budget_status" > "$docker_summary_file"
fi

finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
finished_epoch=$(date +%s)
duration_seconds=$((finished_epoch - started_epoch))

status=failed
if [[ $exit_code -eq 0 ]]; then
  status=passed
elif [[ $exit_code -eq 124 ]]; then
  status=timed_out
fi

printf '{"lane":"%s","status":"%s","exit_code":%d,"started_at":"%s","finished_at":"%s","duration_seconds":%d,"wall_timeout":"%s"}\n' \
  "$lane" "$status" "$exit_code" "$started_at" "$finished_at" "$duration_seconds" "$wall_timeout" > "$timing_file"

echo "[$lane] $status after ${duration_seconds}s (exit $exit_code)"
exit "$exit_code"
