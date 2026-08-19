#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
root_pids=()
observer_pids=()

cleanup() {
  local pid
  for pid in "${observer_pids[@]}" "${root_pids[@]}"; do
    [[ -n $pid ]] || continue
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$test_dir"
}
trap cleanup EXIT

write_fake_process() {
  local proc_root=$1
  local pid=$2
  local child_pid=$3

  mkdir -p "$proc_root/$pid/task/$pid" "$proc_root/$child_pid"
  printf '1.00 1.00\n' > "$proc_root/uptime"
  printf '%s (observer-root) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n' "$pid" \
    > "$proc_root/$pid/stat"
  printf '10 4 0 0 0 0 0\n' > "$proc_root/$pid/statm"
  printf '%s\n' "$child_pid" > "$proc_root/$pid/task/$pid/children"
  printf '%s (transient-child) S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n' "$child_pid" \
    > "$proc_root/$child_pid/stat"
}

cat > "$test_dir/remove-child-stat" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if (( $1 >= 900001 && $1 <= 900005 )); then
  rm -f "$2"
fi
EOF
chmod +x "$test_dir/remove-child-stat"

# Five observers start together. Each process tree contains a child whose proc
# entry passes the readability check and is then removed by the coordinated
# read hook before the file is opened. Process churn is normal measurement
# noise and must not terminate the observer.
for index in 1 2 3 4 5; do
  sleep 1 &
  root_pid=$!
  root_pids+=("$root_pid")
  proc_root="$test_dir/proc-$index"
  child_pid=$((900000 + index))
  write_fake_process "$proc_root" "$root_pid" "$child_pid"
  CI_RESOURCE_PROC_ROOT="$proc_root" \
    CI_RESOURCE_PROC_READ_HOOK="$test_dir/remove-child-stat" \
    "$repo_root/scripts/observe-ci-process.sh" "$root_pid" \
    "$test_dir/samples-$index.tsv" "$test_dir/summary-$index.json" "" 0.01 \
    > "$test_dir/observer-$index.log" 2>&1 &
  observer_pids+=("$!")
done

observer_failed=0
for pid in "${observer_pids[@]}"; do
  if ! wait "$pid"; then
    observer_failed=1
  fi
done
observer_pids=()
for pid in "${root_pids[@]}"; do
  wait "$pid" 2>/dev/null || true
done
root_pids=()
if [[ $observer_failed -ne 0 ]]; then
  echo "a resource observer died on a disappearing proc entry" >&2
  exit 1
fi
for index in 1 2 3 4 5; do
  test -s "$test_dir/summary-$index.json"
  samples=$(sed -n 's/.*"samples":\([0-9]*\).*/\1/p' "$test_dir/summary-$index.json")
  if [[ ! $samples =~ ^[1-9][0-9]*$ ]]; then
    echo "observer $index published an invalid sample count" >&2
    exit 1
  fi
done

# A live owner container without a readable Docker stats/cgroup row remains a
# hard failure. The proc-race tolerance above must not turn missing container
# resource data into a green measurement.
mkdir -p "$test_dir/bin"
cat > "$test_dir/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case ${1:-} in
  ps) printf '%064d\n' 1 ;;
  stats) exit 1 ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$test_dir/bin/docker"

sleep 2 &
root_pid=$!
root_pids+=("$root_pid")
observer_exit=0
PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/observe-ci-process.sh" "$root_pid" \
  "$test_dir/missing-cgroup-samples.tsv" "$test_dir/missing-cgroup-summary.json" \
  owner-missing-cgroup 0.01 > "$test_dir/missing-cgroup.log" 2>&1 || observer_exit=$?
kill -TERM "$root_pid" 2>/dev/null || true
wait "$root_pid" 2>/dev/null || true
root_pids=()
if [[ $observer_exit -eq 0 ]]; then
  echo "live owner container without cgroup data produced a passing observer" >&2
  exit 1
fi
if [[ -e $test_dir/missing-cgroup-summary.json ]]; then
  echo "failed cgroup measurement published a valid summary" >&2
  exit 1
fi
grep -q 'could not read owner container cgroups' "$test_dir/missing-cgroup.log"

echo "observe-ci-process tests passed"
