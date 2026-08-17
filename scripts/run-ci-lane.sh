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
stop_docker_events() {
  if [[ -n $docker_events_pid ]]; then
    kill "$docker_events_pid" 2>/dev/null || true
    wait "$docker_events_pid" 2>/dev/null || true
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
fi

echo "[$lane] starting (wall timeout: $wall_timeout)"
set +e
"$timeout_bin" --kill-after=30s "$wall_timeout" "$@" 2>&1 | tee "$log_file"
exit_code=${PIPESTATUS[0]}
set -e

stop_docker_events
trap - EXIT

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

if [[ -n $docker_event_scope ]]; then
  container_starts=$(grep -c '"Action":"start"' "$docker_events_file" || true)
  container_restarts=$(grep -c '"Action":"restart"' "$docker_events_file" || true)
  printf '{"lane":"%s","scope":"%s","container_start_events":%d,"container_restart_events":%d}\n' \
    "$lane" "$docker_event_scope" "$container_starts" "$container_restarts" > "$docker_summary_file"
fi

echo "[$lane] $status after ${duration_seconds}s (exit $exit_code)"
exit "$exit_code"
