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
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
started_epoch=$(date +%s)

echo "[$lane] starting (wall timeout: $wall_timeout)"
set +e
"$timeout_bin" --kill-after=30s "$wall_timeout" "$@" 2>&1 | tee "$log_file"
exit_code=${PIPESTATUS[0]}
set -e

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
