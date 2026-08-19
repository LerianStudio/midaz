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
setsid_bin=$(command -v setsid || true)
if [[ -z $setsid_bin ]]; then
  echo "required CI lane '$lane' needs setsid to isolate its process group" >&2
  exit 2
fi

report_dir=${CI_REPORT_DIR:-reports/ci}
mkdir -p "$report_dir"

log_file="$report_dir/$lane.log"
timing_file="$report_dir/$lane-timing.json"
docker_events_file="$report_dir/$lane-docker-events.jsonl"
docker_summary_file="$report_dir/$lane-docker-summary.json"
docker_survivors_file="$report_dir/$lane-docker-survivors.tsv"
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
started_epoch=$(date +%s)

docker_events_pid=
docker_events_failed=0
docker_owner=
lane_pid=
lane_pgid=
lane_active=0
resource_observer_pid=
requested_signal=TERM

stop_docker_events() {
  if [[ -n $docker_events_pid ]]; then
    if kill -0 "$docker_events_pid" 2>/dev/null; then
      kill -TERM "$docker_events_pid" 2>/dev/null || true
      wait "$docker_events_pid" 2>/dev/null || true
    else
      wait "$docker_events_pid" 2>/dev/null || true
      docker_events_failed=1
    fi
    docker_events_pid=
  fi
}

# ShellCheck cannot see calls made from the EXIT trap body.
# shellcheck disable=SC2317
wait_for_lane_group() {
  local max_seconds=${CI_SIGNAL_CLEANUP_TIMEOUT_SECONDS:-10}
  if [[ ! $max_seconds =~ ^[1-9][0-9]*$ ]]; then
    max_seconds=10
  fi
  local deadline=$((SECONDS + max_seconds))
  while [[ -n $lane_pgid ]] && kill -0 -- "-$lane_pgid" 2>/dev/null; do
    if (( SECONDS >= deadline )); then
      kill -KILL -- "-$lane_pgid" 2>/dev/null || true
      break
    fi
    sleep 0.1
  done
  if [[ -n $lane_pid ]]; then
    wait "$lane_pid" 2>/dev/null || true
  fi
  lane_active=0
}

# ShellCheck cannot see functions invoked through trap strings.
# shellcheck disable=SC2317
cleanup_on_exit() {
  local status=$?
  trap - EXIT TERM INT
  if [[ $lane_active -eq 1 ]]; then
    if [[ -n $lane_pgid ]]; then
      kill -"$requested_signal" -- "-$lane_pgid" 2>/dev/null || true
    elif [[ -n $lane_pid ]]; then
      kill -"$requested_signal" "$lane_pid" 2>/dev/null || true
    fi
    wait_for_lane_group
  fi
  if [[ -n $resource_observer_pid ]]; then
    if kill -0 "$resource_observer_pid" 2>/dev/null; then
      kill -TERM "$resource_observer_pid" 2>/dev/null || true
    fi
    wait "$resource_observer_pid" 2>/dev/null || true
  fi
  if [[ -n ${lane_ready_file:-} ]]; then
    rm -f "$lane_ready_file" "$lane_start_file"
  fi
  stop_docker_events
  exit "$status"
}

trap cleanup_on_exit EXIT
trap 'requested_signal=TERM; exit 143' TERM
trap 'requested_signal=INT; exit 130' INT

docker_event_scope=${CI_CAPTURE_DOCKER_EVENTS:-}
require_owner_events=${CI_REQUIRE_DOCKER_OWNER_EVENTS:-0}
if [[ $require_owner_events != 0 && $require_owner_events != 1 ]]; then
  echo "CI_REQUIRE_DOCKER_OWNER_EVENTS must be 0 or 1" >&2
  exit 2
fi
if [[ $require_owner_events == 1 && $docker_event_scope != owner ]]; then
  echo "CI_REQUIRE_DOCKER_OWNER_EVENTS requires CI_CAPTURE_DOCKER_EVENTS=owner" >&2
  exit 2
fi

docker_bin=
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

  "$docker_bin" events "${docker_event_filters[@]}" --format '{{json .}}' \
    > "$docker_events_file" 2> "$report_dir/$lane-docker-events.log" &
  docker_events_pid=$!

  sleep 1
  if ! kill -0 "$docker_events_pid" 2>/dev/null; then
    wait "$docker_events_pid" 2>/dev/null || true
    docker_events_pid=
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
lane_ready_file="$report_dir/$lane-process-group.ready"
lane_start_file="$report_dir/$lane-process-group.start"
rm -f "$lane_ready_file" "$lane_start_file"
lane_started_ns=$(date +%s%N)
set +e
# Expanded by the dedicated Bash process, not by this runner.
# shellcheck disable=SC2016
"$setsid_bin" bash -c '
  cpu_time_file=$1
  ready_file=$2
  start_file=$3
  shift 3
  exec 3>&2
  : > "$ready_file"
  while [[ ! -e $start_file ]]; do sleep 0.01; done
  TIMEFORMAT="MIDAZ_CPU_TIME %U %S"
  { time "$@" 2>&3; } 2> "$cpu_time_file"
' _ "$cpu_time_file" "$lane_ready_file" "$lane_start_file" \
  "$timeout_bin" --foreground --kill-after=30s "$wall_timeout" "$@" \
  > >(tee "$log_file") 2>&1 &
lane_pid=$!
set -e
lane_active=1
for _ in {1..100}; do
  [[ -e $lane_ready_file ]] && break
  if ! kill -0 "$lane_pid" 2>/dev/null; then
    break
  fi
  sleep 0.01
done
lane_pgid=$(ps -o pgid= -p "$lane_pid" | tr -d '[:space:]' || true)
if [[ ! $lane_pgid =~ ^[1-9][0-9]*$ || $lane_pgid -ne $lane_pid ]]; then
  echo "required CI lane '$lane' could not create a dedicated process group" >&2
  exit 2
fi
: > "$lane_start_file"

resource_observer_failed=0
resource_raw_file="$report_dir/$lane-resources-raw.json"
resource_summary_file="$report_dir/$lane-resources.json"
resource_samples_file="$report_dir/$lane-resource-samples.tsv"
if [[ $capture_resources == 1 ]]; then
  resource_observer=${CI_RESOURCE_OBSERVER:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/observe-ci-process.sh}
  if [[ ! -x $resource_observer ]]; then
    echo "required CI lane '$lane' resource observer is not executable: $resource_observer" >&2
    exit 2
  fi
  "$resource_observer" "$lane_pid" "$resource_samples_file" "$resource_raw_file" \
    "$docker_owner" &
  resource_observer_pid=$!
fi

set +e
wait "$lane_pid"
exit_code=$?
set -e
lane_active=0
rm -f "$lane_ready_file" "$lane_start_file"
lane_finished_ns=$(date +%s%N)

if [[ -n $resource_observer_pid ]]; then
  if ! wait "$resource_observer_pid"; then
    resource_observer_failed=1
  fi
  resource_observer_pid=
  if [[ $resource_observer_failed -eq 1 || ! -s $resource_raw_file ]]; then
    echo "required CI lane '$lane' resource observer failed before publishing a summary" >&2
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  else
    measurement_scope=$(sed -n 's/.*"measurement_scope":"\([^"]*\)".*/\1/p' "$resource_raw_file")
    peak_process_rss_mb=$(sed -n 's/.*"peak_process_rss_mb":\([0-9]*\).*/\1/p' "$resource_raw_file")
    peak_container_rss_mb=$(sed -n 's/.*"peak_container_rss_mb":\([0-9]*\).*/\1/p' "$resource_raw_file")
    peak_rss_mb=$(sed -n 's/.*"peak_rss_mb":\([0-9]*\).*/\1/p' "$resource_raw_file")
    average_container_cpu_percent=$(sed -n 's/.*"average_container_cpu_percent":\([0-9.]*\).*/\1/p' "$resource_raw_file")
    resource_samples=$(sed -n 's/.*"samples":\([0-9]*\).*/\1/p' "$resource_raw_file")
    read -r cpu_marker user_cpu_seconds system_cpu_seconds < "$cpu_time_file" || true
    if [[ $cpu_marker != MIDAZ_CPU_TIME || ! $user_cpu_seconds =~ ^[0-9]+([.][0-9]+)?$ || \
      ! $system_cpu_seconds =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      echo "required CI lane '$lane' could not read aggregate process CPU time" >&2
      if [[ $exit_code -eq 0 ]]; then
        exit_code=2
      fi
    elif [[ -z $measurement_scope || -z $peak_process_rss_mb || \
      -z $peak_container_rss_mb || -z $peak_rss_mb || \
      -z $average_container_cpu_percent || -z $resource_samples ]]; then
      echo "required CI lane '$lane' resource observer published an invalid summary" >&2
      if [[ $exit_code -eq 0 ]]; then
        exit_code=2
      fi
    else
      read -r process_cpu_seconds average_process_cpu_percent \
        container_cpu_seconds_estimate average_cpu_percent < <(
        awk -v user="$user_cpu_seconds" -v syscpu="$system_cpu_seconds" \
          -v container_average="$average_container_cpu_percent" \
          -v started="$lane_started_ns" -v finished="$lane_finished_ns" '
          BEGIN {
            process_cpu = user + syscpu
            wall = (finished - started) / 1000000000
            process_average = wall > 0 ? process_cpu * 100 / wall : 0
            container_cpu = wall * container_average / 100
            printf "%.3f %.2f %.3f %.2f\n", process_cpu, process_average,
              container_cpu, process_average + container_average
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
      printf '{"lane":"%s","measurement_scope":"%s","samples":%d,"process_cpu_seconds":%s,"container_cpu_seconds_estimate":%s,"average_process_cpu_percent":%s,"average_container_cpu_percent":%s,"average_cpu_percent":%s,"peak_process_rss_mb":%d,"peak_container_rss_mb":%d,"peak_rss_mb":%d,"max_average_cpu_percent":"%s","max_rss_mb":"%s","budget_status":"%s"}\n' \
        "$lane" "$measurement_scope" "$resource_samples" "$process_cpu_seconds" \
        "$container_cpu_seconds_estimate" "$average_process_cpu_percent" \
        "$average_container_cpu_percent" "$average_cpu_percent" \
        "$peak_process_rss_mb" "$peak_container_rss_mb" "$peak_rss_mb" \
        "${CI_MAX_AVERAGE_CPU_PERCENT:-}" "${CI_MAX_RSS_MB:-}" "$budget_status" \
        > "$resource_summary_file"
      if [[ $budget_status == exceeded && $exit_code -eq 0 ]]; then
        echo "required CI lane '$lane' exceeded its total CPU or RSS budget" >&2
        exit_code=2
      fi
    fi
  fi
fi

ryuk_survivors=0
non_ryuk_survivors=0
if [[ $docker_event_scope == owner ]]; then
  cleanup_timeout=${CI_DOCKER_CLEANUP_TIMEOUT_SECONDS:-30}
  if [[ ! $cleanup_timeout =~ ^[1-9][0-9]*$ ]]; then
    echo "CI_DOCKER_CLEANUP_TIMEOUT_SECONDS must be a positive integer" >&2
    exit_code=2
    cleanup_timeout=1
  fi
  cleanup_deadline=$((SECONDS + cleanup_timeout))
  survivors_tmp="$report_dir/$lane-docker-survivors.tmp"
  while :; do
    if ! "$docker_bin" ps -a \
      --filter "label=org.testcontainers.sessionId=$docker_owner" \
      --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Labels}}' > "$survivors_tmp"; then
      echo "required CI lane '$lane' could not inspect owner container cleanup" >&2
      if [[ $exit_code -eq 0 ]]; then
        exit_code=2
      fi
      : > "$survivors_tmp"
      break
    fi
    non_ryuk_survivors=$(awk -F '\t' '
      {
        identity = tolower($2 " " $3 " " $4)
        if (identity !~ /testcontainers\/ryuk/ && identity !~ /(^|[[:space:]])reaper[_-]/ && identity !~ /org\.testcontainers\.ryuk=true/) count++
      }
      END { print count + 0 }
    ' "$survivors_tmp")
    if [[ $non_ryuk_survivors -eq 0 || $SECONDS -ge $cleanup_deadline ]]; then
      break
    fi
    sleep 1
  done
  {
    printf 'container_id\tname\timage\tclassification\n'
    awk -F '\t' 'BEGIN { OFS="\t" }
      {
        identity = tolower($2 " " $3 " " $4)
        classification = "non-ryuk"
        if (identity ~ /testcontainers\/ryuk/ || identity ~ /(^|[[:space:]])reaper[_-]/ || identity ~ /org\.testcontainers\.ryuk=true/) classification = "ryuk"
        print $1, $2, $3, classification
      }
    ' "$survivors_tmp"
  } > "$docker_survivors_file"
  ryuk_survivors=$(awk -F '\t' '$4 == "ryuk" { count++ } END { print count + 0 }' "$docker_survivors_file")
  non_ryuk_survivors=$(awk -F '\t' '$4 == "non-ryuk" { count++ } END { print count + 0 }' "$docker_survivors_file")
  rm -f "$survivors_tmp"
  if [[ $non_ryuk_survivors -gt 0 ]]; then
    echo "required CI lane '$lane' left $non_ryuk_survivors non-Ryuk owner container(s) after ${cleanup_timeout}s" >&2
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  fi
fi

stop_docker_events

if [[ $docker_events_failed -eq 1 ]]; then
  echo "required CI lane '$lane' Docker event observer exited before the lane finished" >&2
  if [[ $exit_code -eq 0 ]]; then
    exit_code=2
  fi
fi

if [[ -n $docker_event_scope && $docker_events_failed -eq 0 ]]; then
  read -r container_starts container_restarts peak_live_containers owner_mismatches < <(
    awk -v owner="$docker_owner" -v require_owner="$require_owner_events" '
      index($0, "\"Action\":\"start\"") { starts++; live++; if (live > peak) peak = live; tracked=1 }
      index($0, "\"Action\":\"restart\"") { restarts++; tracked=1 }
      index($0, "\"Action\":\"die\"") { if (live > 0) live--; tracked=1 }
      {
        if (tracked && require_owner == 1 && index($0, "\"org.testcontainers.sessionId\":\"" owner "\"") == 0) mismatches++
        tracked=0
      }
      END { printf "%d %d %d %d\n", starts + 0, restarts + 0, peak + 0, mismatches + 0 }
    ' "$docker_events_file"
  )
  observer_status=valid
  if [[ $require_owner_events == 1 && $container_starts -eq 0 ]]; then
    echo "required CI lane '$lane' observed zero owner-attributed container starts" >&2
    observer_status=invalid
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  fi
  if [[ $owner_mismatches -gt 0 ]]; then
    echo "required CI lane '$lane' observed $owner_mismatches incorrectly attributed Docker event(s)" >&2
    observer_status=invalid
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  fi
  container_budget_status=within
  if [[ -n ${CI_MAX_PEAK_CONTAINERS:-} ]] && \
    awk -v value="$peak_live_containers" -v limit="$CI_MAX_PEAK_CONTAINERS" 'BEGIN { exit !(value > limit) }'; then
    container_budget_status=exceeded
    if [[ $exit_code -eq 0 ]]; then
      exit_code=2
    fi
  fi
  printf '{"lane":"%s","scope":"%s","owner":"%s","observer_status":"%s","container_start_events":%d,"container_restart_events":%d,"peak_live_containers":%d,"owner_mismatches":%d,"ryuk_survivors":%d,"non_ryuk_survivors":%d,"max_peak_containers":"%s","budget_status":"%s"}\n' \
    "$lane" "$docker_event_scope" "$docker_owner" "$observer_status" \
    "$container_starts" "$container_restarts" "$peak_live_containers" \
    "$owner_mismatches" "$ryuk_survivors" "$non_ryuk_survivors" \
    "${CI_MAX_PEAK_CONTAINERS:-}" "$container_budget_status" > "$docker_summary_file"
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
trap - EXIT TERM INT
exit "$exit_code"
