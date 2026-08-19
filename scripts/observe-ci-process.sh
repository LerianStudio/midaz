#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "usage: $0 <root-pid> <samples.tsv> <summary.json> [interval-seconds]" >&2
  exit 2
fi

root_pid=$1
samples_file=$2
summary_file=$3
interval=${4:-0.5}

if [[ ! $root_pid =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid resource observer root pid: $root_pid" >&2
  exit 2
fi
if [[ ! $interval =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "invalid resource observer interval: $interval" >&2
  exit 2
fi

mkdir -p "$(dirname "$samples_file")" "$(dirname "$summary_file")"
printf 'epoch_seconds\trss_kb\n' > "$samples_file"

peak_rss_kb=0
sample_count=0
page_size=$(getconf PAGESIZE) || {
  echo "resource observer could not read the system page size" >&2
  exit 1
}
if [[ ! -r /proc/uptime ]]; then
  echo "resource observer requires Linux procfs" >&2
  exit 1
fi

while :; do
  root_alive=0
  if kill -0 "$root_pid" 2>/dev/null; then
    root_alive=1
  fi
  if [[ $root_alive -eq 0 && $sample_count -gt 0 ]]; then
    break
  fi

  rss_kb=0
  declare -A visited_pids=()
  process_queue=("$root_pid")
  queue_index=0
  while (( queue_index < ${#process_queue[@]} )); do
    pid=${process_queue[$queue_index]}
    queue_index=$((queue_index + 1))
    if [[ -n ${visited_pids[$pid]+present} ]]; then
      continue
    fi
    visited_pids[$pid]=1

    stat_file=/proc/$pid/stat
    [[ -r $stat_file ]] || continue
    stat_line=$(<"$stat_file") || continue
    # Everything after the final ')' starts at procfs stat field 3. The
    # executable name itself may contain spaces or parentheses.
    stat_fields=${stat_line##*) }
    read -r -a fields <<< "$stat_fields"
    if (( ${#fields[@]} < 20 )); then
      continue
    fi
    statm_file=/proc/$pid/statm
    if [[ -r $statm_file ]]; then
      read -r _ process_rss_pages _ < "$statm_file" || process_rss_pages=0
      if (( process_rss_pages > 0 )); then
        rss_kb=$((rss_kb + (process_rss_pages * page_size + 1023) / 1024))
      fi
    fi

    children_file=/proc/$pid/task/$pid/children
    if [[ -r $children_file ]]; then
      child_pids=()
      read -r -a child_pids < "$children_file" || true
      process_queue+=("${child_pids[@]}")
    fi
  done

  printf '%s\t%s\n' "$(date +%s)" "$rss_kb" >> "$samples_file"
  sample_count=$((sample_count + 1))
  if (( rss_kb > peak_rss_kb )); then
    peak_rss_kb=$rss_kb
  fi

  if [[ $root_alive -eq 0 ]]; then
    break
  fi
  sleep "$interval"
done

peak_rss_mb=$(((peak_rss_kb + 1023) / 1024))
summary_tmp=$summary_file.tmp.$$
printf '{"root_pid":%d,"samples":%d,"peak_rss_mb":%d}\n' \
  "$root_pid" "$sample_count" "$peak_rss_mb" > "$summary_tmp"
mv "$summary_tmp" "$summary_file"
