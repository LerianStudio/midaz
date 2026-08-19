#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if [[ $# -lt 3 || $# -gt 5 ]]; then
  echo "usage: $0 <root-pid> <samples.tsv> <summary.json> [docker-owner] [interval-seconds]" >&2
  exit 2
fi

root_pid=$1
samples_file=$2
summary_file=$3
docker_owner=${4:-}
interval=${5:-0.5}

if [[ ! $root_pid =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid resource observer root pid: $root_pid" >&2
  exit 2
fi
if [[ -n $docker_owner && ! $docker_owner =~ ^[a-zA-Z0-9._-]+$ ]]; then
  echo "invalid resource observer Docker owner: $docker_owner" >&2
  exit 2
fi
if [[ ! $interval =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "invalid resource observer interval: $interval" >&2
  exit 2
fi

mkdir -p "$(dirname "$samples_file")" "$(dirname "$summary_file")"
printf 'epoch_nanoseconds\tprocess_rss_kb\tcontainer_rss_kb\ttotal_rss_kb\tcontainer_cpu_percent\n' > "$samples_file"

peak_process_rss_kb=0
peak_container_rss_kb=0
peak_total_rss_kb=0
container_cpu_percent_seconds=0
sample_count=0
observer_started_ns=$(date +%s%N)
previous_sample_ns=$observer_started_ns
page_size=$(getconf PAGESIZE) || {
  echo "resource observer could not read the system page size" >&2
  exit 1
}
if [[ ! -r /proc/uptime ]]; then
  echo "resource observer requires Linux procfs" >&2
  exit 1
fi

docker_bin=
stats_fallback_dir=
if [[ -n $docker_owner ]]; then
  docker_bin=$(command -v docker || true)
  if [[ -z $docker_bin ]]; then
    echo "resource observer cannot measure owner containers because docker is unavailable" >&2
    exit 1
  fi
  stats_fallback_dir=$(mktemp -d "$(dirname "$samples_file")/.resource-stats.XXXXXX")
fi

cleanup_stats_fallback() {
  if [[ -n $stats_fallback_dir ]]; then
    rm -rf -- "$stats_fallback_dir"
  fi
}
trap cleanup_stats_fallback EXIT

read_container_resources() {
  local container_ids current_ids final_ids stats stats_ids container_id missing_stats stats_complete
  local fallback_stats row row_id row_file attempt index fallback_pid
  local -a container_id_array fallback_ids fallback_files fallback_pids
  stats=
  stats_complete=0
  # Docker obtains these values from the containers' cgroups. Containers can
  # legitimately disappear between `ps` and `stats` during teardown. Docker
  # then returns partial rows and a non-zero exit. Keep those rows only when
  # every container that survived the snapshot has a row; a live container
  # with no cgroup data remains a hard observer failure.
  for attempt in 1 2 3; do
    container_ids=$("$docker_bin" ps -q --no-trunc \
      --filter "label=org.testcontainers.sessionId=$docker_owner") || return 1
    if [[ -z $container_ids ]]; then
      printf '0 0\n'
      return 0
    fi
    mapfile -t container_id_array <<< "$container_ids"
    stats=
    if stats=$("$docker_bin" stats --no-stream --no-trunc \
      --format '{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}' "${container_id_array[@]}" 2>/dev/null); then
      :
    fi

    current_ids=$("$docker_bin" ps -q --no-trunc \
      --filter "label=org.testcontainers.sessionId=$docker_owner") || return 1
    stats_ids=$(awk -F '\t' 'NF >= 3 { print $1 }' <<< "$stats")
    missing_stats=0
    for container_id in "${container_id_array[@]}"; do
      if grep -Fxq "$container_id" <<< "$current_ids" && \
        ! grep -Fxq "$container_id" <<< "$stats_ids"; then
        missing_stats=1
        break
      fi
    done
    if [[ $missing_stats -eq 0 ]]; then
      if [[ -z $stats ]]; then
        printf '0 0\n'
        return 0
      fi
      stats_complete=1
      break
    fi

    # A single disappearing container can make Docker discard every row from
    # a batch request. Read the refreshed owner set concurrently so one slow
    # cgroup does not serialize the rest. A container that remains live but
    # still has no valid row keeps this sample fail-closed.
    fallback_ids=()
    if [[ -n $current_ids ]]; then
      mapfile -t fallback_ids <<< "$current_ids"
    fi
    fallback_files=()
    fallback_pids=()
    for container_id in "${fallback_ids[@]}"; do
      row_file=$stats_fallback_dir/$attempt-$container_id.tsv
      fallback_files+=("$row_file")
      (
        "$docker_bin" stats --no-stream --no-trunc \
          --format '{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}' "$container_id" \
          > "$row_file" 2>/dev/null
      ) &
      fallback_pids+=("$!")
    done
    for fallback_pid in "${fallback_pids[@]}"; do
      if wait "$fallback_pid"; then
        :
      fi
    done

    final_ids=$("$docker_bin" ps -q --no-trunc \
      --filter "label=org.testcontainers.sessionId=$docker_owner") || return 1
    fallback_stats=
    missing_stats=0
    for index in "${!fallback_ids[@]}"; do
      container_id=${fallback_ids[$index]}
      row_file=${fallback_files[$index]}
      row=
      if [[ -s $row_file ]]; then
        row=$(<"$row_file")
      fi
      row_id=$(awk -F '\t' 'NF >= 3 { print $1; exit }' <<< "$row")
      if [[ $row_id == "$container_id" ]]; then
        if [[ -n $fallback_stats ]]; then
          fallback_stats+=$'\n'
        fi
        fallback_stats+=$row
      elif grep -Fxq "$container_id" <<< "$final_ids"; then
        missing_stats=1
      fi
    done
    if [[ $missing_stats -eq 0 ]]; then
      if [[ -z $fallback_stats ]]; then
        printf '0 0\n'
        return 0
      fi
      stats=$fallback_stats
      stats_complete=1
      break
    fi
    if ! kill -0 "$root_pid" 2>/dev/null; then
      printf '0 0\n'
      return 0
    fi
    sleep 0.05
  done
  [[ $stats_complete -eq 1 ]] || return 1

  awk -F '\t' '
    function kib(value, number, unit) {
      number = value + 0
      unit = value
      sub(/^[0-9.]+/, "", unit)
      if (unit == "B") return number / 1024
      if (unit == "kB" || unit == "KB" || unit == "KiB") return number
      if (unit == "MB" || unit == "MiB") return number * 1024
      if (unit == "GB" || unit == "GiB") return number * 1024 * 1024
      if (unit == "TB" || unit == "TiB") return number * 1024 * 1024 * 1024
      exit 2
    }
    NF >= 3 {
      cpu = $2
      gsub(/%/, "", cpu)
      split($3, memory, /[[:space:]]+/)
      cpu_total += cpu + 0
      memory_total += kib(memory[1])
    }
    END { printf "%.2f %.0f\n", cpu_total + 0, memory_total + 0 }
  ' <<< "$stats"
}

while :; do
  root_alive=0
  if kill -0 "$root_pid" 2>/dev/null; then
    root_alive=1
  fi
  if [[ $root_alive -eq 0 && $sample_count -gt 0 ]]; then
    break
  fi

  process_rss_kb=0
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
    stat_fields=${stat_line##*) }
    read -r -a fields <<< "$stat_fields"
    if (( ${#fields[@]} < 20 )); then
      continue
    fi
    statm_file=/proc/$pid/statm
    if [[ -r $statm_file ]]; then
      read -r _ process_rss_pages _ < "$statm_file" || process_rss_pages=0
      if (( process_rss_pages > 0 )); then
        process_rss_kb=$((process_rss_kb + (process_rss_pages * page_size + 1023) / 1024))
      fi
    fi

    children_file=/proc/$pid/task/$pid/children
    if [[ -r $children_file ]]; then
      child_pids=()
      read -r -a child_pids < "$children_file" || true
      process_queue+=("${child_pids[@]}")
    fi
  done

  container_cpu_percent=0
  container_rss_kb=0
  if [[ -n $docker_owner ]]; then
    if ! read -r container_cpu_percent container_rss_kb < <(read_container_resources); then
      echo "resource observer could not read owner container cgroups" >&2
      exit 1
    fi
  fi
  total_rss_kb=$((process_rss_kb + container_rss_kb))
  sample_ns=$(date +%s%N)
  sample_elapsed_seconds=$(awk -v current="$sample_ns" -v previous="$previous_sample_ns" \
    'BEGIN { printf "%.9f", (current - previous) / 1000000000 }')
  container_cpu_percent_seconds=$(awk -v total="$container_cpu_percent_seconds" \
    -v sample="$container_cpu_percent" -v elapsed="$sample_elapsed_seconds" \
    'BEGIN { printf "%.6f", total + sample * elapsed }')
  previous_sample_ns=$sample_ns
  printf '%s\t%s\t%s\t%s\t%s\n' "$sample_ns" "$process_rss_kb" \
    "$container_rss_kb" "$total_rss_kb" "$container_cpu_percent" >> "$samples_file"
  sample_count=$((sample_count + 1))
  (( process_rss_kb > peak_process_rss_kb )) && peak_process_rss_kb=$process_rss_kb
  (( container_rss_kb > peak_container_rss_kb )) && peak_container_rss_kb=$container_rss_kb
  (( total_rss_kb > peak_total_rss_kb )) && peak_total_rss_kb=$total_rss_kb

  if [[ $root_alive -eq 0 ]]; then
    break
  fi
  sleep "$interval"
done

peak_process_rss_mb=$(((peak_process_rss_kb + 1023) / 1024))
peak_container_rss_mb=$(((peak_container_rss_kb + 1023) / 1024))
peak_rss_mb=$(((peak_total_rss_kb + 1023) / 1024))
observer_finished_ns=$(date +%s%N)
average_container_cpu_percent=$(awk -v total="$container_cpu_percent_seconds" \
  -v started="$observer_started_ns" -v finished="$observer_finished_ns" '
  BEGIN {
    elapsed = (finished - started) / 1000000000
    printf "%.2f", (elapsed > 0 ? total / elapsed : 0)
  }
')
measurement_scope=process_tree
if [[ -n $docker_owner ]]; then
  measurement_scope=process_tree_plus_owner_containers
fi
summary_tmp=$summary_file.tmp.$$
printf '{"root_pid":%d,"measurement_scope":"%s","docker_owner":"%s","samples":%d,"peak_process_rss_mb":%d,"peak_container_rss_mb":%d,"peak_rss_mb":%d,"average_container_cpu_percent":%s}\n' \
  "$root_pid" "$measurement_scope" "$docker_owner" "$sample_count" \
  "$peak_process_rss_mb" "$peak_container_rss_mb" "$peak_rss_mb" \
  "$average_container_cpu_percent" > "$summary_tmp"
mv "$summary_tmp" "$summary_file"
