#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" success 5s bash -c 'printf "lane ran\n"'
grep -q '"status":"passed"' "$test_dir/success-timing.json"
grep -q 'lane ran' "$test_dir/success.log"

status=0
CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" failure 5s bash -c 'exit 7' || status=$?
if [[ $status -ne 7 ]]; then
  echo "failure lane returned $status, want 7" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/failure-timing.json"
grep -q '"exit_code":7' "$test_dir/failure-timing.json"

status=0
CI_REPORT_DIR="$test_dir" "$repo_root/scripts/run-ci-lane.sh" bounded 1s bash -c 'sleep 5' || status=$?
if [[ $status -ne 124 ]]; then
  echo "bounded lane returned $status, want timeout exit 124" >&2
  exit 1
fi
grep -q '"status":"timed_out"' "$test_dir/bounded-timing.json"

# TERM must reach only the lane's dedicated process group, including the
# command's child, and the runner must wait until that group is gone.
cat > "$test_dir/signal-lane.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
trap 'printf "leader-term\n" >> "$SIGNAL_LOG"; exit 0' TERM
bash -c 'trap '\''printf "child-term\n" >> "$SIGNAL_LOG"; exit 0'\'' TERM; printf "%s\n" "$$" > "$CHILD_PID_FILE"; while :; do sleep 1; done' &
child_pid=$!
printf '%s\n' "$child_pid" > "$CHILD_PID_FILE"
wait "$child_pid"
EOF
chmod +x "$test_dir/signal-lane.sh"
SIGNAL_LOG="$test_dir/signal.log" CHILD_PID_FILE="$test_dir/signal-child.pid" \
  CI_REPORT_DIR="$test_dir" \
  "$repo_root/scripts/run-ci-lane.sh" signalled 30s "$test_dir/signal-lane.sh" &
runner_pid=$!
for _ in {1..50}; do
  [[ -s $test_dir/signal-child.pid ]] && break
  sleep 0.1
done
[[ -s $test_dir/signal-child.pid ]]
child_pid=$(<"$test_dir/signal-child.pid")
kill -TERM "$runner_pid"
status=0
wait "$runner_pid" || status=$?
if [[ $status -ne 143 ]]; then
  echo "TERM-forwarded lane returned $status, want 143" >&2
  exit 1
fi
grep -q 'child-term' "$test_dir/signal.log"
if kill -0 "$child_pid" 2>/dev/null; then
  echo "TERM-forwarded lane left child $child_pid alive" >&2
  exit 1
fi

mkdir -p "$test_dir/bin"
cat > "$test_dir/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${FAKE_DOCKER_ARGS_FILE:-/dev/null}"
command=${1:-}
shift || true
case $command in
  events)
    if [[ ${FAKE_DOCKER_MODE:-} == fail-start ]]; then
      exit 9
    fi
    if [[ ${FAKE_DOCKER_MODE:-} == zero-events ]]; then
      exec sleep 30
    fi
    owner=generic-owner
    for arg in "$@"; do
      case $arg in
        label=org.testcontainers.sessionId=*) owner=${arg##*=} ;;
      esac
    done
    if [[ ${FAKE_DOCKER_MODE:-} == wrong-owner ]]; then
      owner=somebody-else
    fi
    printf '%s\n' \
      "{\"Type\":\"container\",\"Action\":\"start\",\"Actor\":{\"Attributes\":{\"org.testcontainers.sessionId\":\"$owner\"}}}" \
      "{\"Type\":\"container\",\"Action\":\"start\",\"Actor\":{\"Attributes\":{\"org.testcontainers.sessionId\":\"$owner\"}}}" \
      "{\"Type\":\"container\",\"Action\":\"restart\",\"Actor\":{\"Attributes\":{\"org.testcontainers.sessionId\":\"$owner\"}}}" \
      "{\"Type\":\"container\",\"Action\":\"die\",\"Actor\":{\"Attributes\":{\"org.testcontainers.sessionId\":\"$owner\"}}}" \
      "{\"Type\":\"container\",\"Action\":\"die\",\"Actor\":{\"Attributes\":{\"org.testcontainers.sessionId\":\"$owner\"}}}"
    if [[ ${FAKE_DOCKER_MODE:-} == fail-mid-lane ]]; then
      sleep 2
      exit 9
    fi
    exec sleep 30
    ;;
  ps)
    if [[ " $* " == *' -q '* || " $* " == *' -aq '* ]]; then
		if [[ ${FAKE_DOCKER_MODE:-} == stats-batch-failure ]]; then
			printf 'resource-container\nryuk-container\n'
			exit 0
		fi
      if [[ ${FAKE_DOCKER_RESOURCE_CONTAINER:-0} == 1 ]]; then
        printf 'resource-container\n'
      fi
      exit 0
    fi
    case ${FAKE_DOCKER_MODE:-} in
      nonryuk-survivor)
        printf 'deadbeef\tpostgres-test\tpostgres:17\torg.testcontainers.sessionId=midaz-owner-test\n'
        ;;
      ryuk-survivor)
        printf 'feedface\treaper_owner\ttestcontainers/ryuk:0.14.0\torg.testcontainers.sessionId=midaz-owner-test\n'
        ;;
    esac
    ;;
  stats)
		if [[ ${FAKE_DOCKER_MODE:-} == stats-batch-failure ]]; then
			if [[ " $* " == *' resource-container '* && " $* " == *' ryuk-container '* ]]; then
				exit 1
			fi
			if [[ " $* " == *' resource-container '* ]]; then
				printf 'resource-container\t25.00%%\t64MiB / 1GiB\n'
			else
				printf 'ryuk-container\t1.00%%\t8MiB / 1GiB\n'
			fi
			exit 0
		fi
		printf 'resource-container\t25.00%%\t64MiB / 1GiB\n'
    ;;
  *)
    echo "unexpected fake Docker command: $command" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$test_dir/bin/docker"

CI_CAPTURE_DOCKER_EVENTS=testcontainers CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" docker-events 5s bash -c 'sleep 1'
grep -q '"scope":"testcontainers"' "$test_dir/docker-events-docker-summary.json"
grep -q '"container_start_events":2' "$test_dir/docker-events-docker-summary.json"
grep -q '"container_restart_events":1' "$test_dir/docker-events-docker-summary.json"
grep -q '"peak_live_containers":2' "$test_dir/docker-events-docker-summary.json"

# Expanded by the nested bash, not this test process.
# shellcheck disable=SC2016
CI_CAPTURE_DOCKER_EVENTS=owner CI_DOCKER_OWNER=midaz-owner-test \
  CI_REQUIRE_DOCKER_OWNER_EVENTS=1 CI_CAPTURE_RESOURCES=1 CI_REPORT_DIR="$test_dir" \
  FAKE_DOCKER_RESOURCE_CONTAINER=1 \
  FAKE_DOCKER_ARGS_FILE="$test_dir/owner-docker-args.txt" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" owner-events 5s bash -c \
    'test "$TESTCONTAINERS_SESSION_ID" = midaz-owner-test; sleep 1'
grep -q 'label=org.testcontainers.sessionId=midaz-owner-test' "$test_dir/owner-docker-args.txt"
grep -q '"owner":"midaz-owner-test"' "$test_dir/owner-events-docker-summary.json"
grep -q '"peak_live_containers":2' "$test_dir/owner-events-docker-summary.json"
grep -q '"non_ryuk_survivors":0' "$test_dir/owner-events-docker-summary.json"
grep -q '"measurement_scope":"process_tree_plus_owner_containers"' "$test_dir/owner-events-resources.json"
grep -q '"peak_process_rss_mb":' "$test_dir/owner-events-resources.json"
grep -q '"peak_container_rss_mb":64' "$test_dir/owner-events-resources.json"
grep -q '"peak_rss_mb":' "$test_dir/owner-events-resources.json"
grep -q '"average_cpu_percent":' "$test_dir/owner-events-resources.json"

# Docker stats can fail the whole batch when one owner container disappears
# during teardown. The observer must fall back to concurrent individual reads
# while continuing to fail closed if a live container has no cgroup data.
FAKE_DOCKER_MODE=stats-batch-failure \
  CI_CAPTURE_DOCKER_EVENTS=owner CI_DOCKER_OWNER=midaz-owner-test \
  CI_REQUIRE_DOCKER_OWNER_EVENTS=1 CI_CAPTURE_RESOURCES=1 CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" resource-batch-failure 5s bash -c 'sleep 1'
grep -q '"peak_container_rss_mb":72' "$test_dir/resource-batch-failure-resources.json"

status=0
FAKE_DOCKER_MODE=zero-events CI_CAPTURE_DOCKER_EVENTS=owner \
  CI_DOCKER_OWNER=midaz-owner-test CI_REQUIRE_DOCKER_OWNER_EVENTS=1 \
  CI_REPORT_DIR="$test_dir" PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" zero-owner-events 5s bash -c 'sleep 1' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane with zero required owner events returned $status, want 2" >&2
  exit 1
fi

status=0
FAKE_DOCKER_MODE=wrong-owner CI_CAPTURE_DOCKER_EVENTS=owner \
  CI_DOCKER_OWNER=midaz-owner-test CI_REQUIRE_DOCKER_OWNER_EVENTS=1 \
  CI_REPORT_DIR="$test_dir" PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" wrong-owner-events 5s bash -c 'sleep 1' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane with incorrectly attributed owner events returned $status, want 2" >&2
  exit 1
fi

status=0
FAKE_DOCKER_MODE=nonryuk-survivor CI_CAPTURE_DOCKER_EVENTS=owner \
  CI_DOCKER_OWNER=midaz-owner-test CI_REQUIRE_DOCKER_OWNER_EVENTS=1 \
  CI_DOCKER_CLEANUP_TIMEOUT_SECONDS=1 CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" leaked-owner-container 5s bash -c 'sleep 1' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane with a non-Ryuk survivor returned $status, want 2" >&2
  exit 1
fi
grep -q $'deadbeef\tpostgres-test\tpostgres:17\tnon-ryuk' "$test_dir/leaked-owner-container-docker-survivors.tsv"
grep -q '"non_ryuk_survivors":1' "$test_dir/leaked-owner-container-docker-summary.json"

FAKE_DOCKER_MODE=ryuk-survivor CI_CAPTURE_DOCKER_EVENTS=owner \
  CI_DOCKER_OWNER=midaz-owner-test CI_REQUIRE_DOCKER_OWNER_EVENTS=1 \
  CI_DOCKER_CLEANUP_TIMEOUT_SECONDS=1 CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" ryuk-only-survivor 5s bash -c 'sleep 1'
grep -q $'feedface\treaper_owner\ttestcontainers/ryuk:0.14.0\tryuk' "$test_dir/ryuk-only-survivor-docker-survivors.tsv"
grep -q '"non_ryuk_survivors":0' "$test_dir/ryuk-only-survivor-docker-summary.json"

status=0
FAKE_DOCKER_MODE=fail-start CI_CAPTURE_DOCKER_EVENTS=testcontainers CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" docker-events-fail-start 5s bash -c 'exit 99' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane with dead Docker observer returned $status, want 2" >&2
  exit 1
fi
if [[ -e $test_dir/docker-events-fail-start-docker-summary.json ]]; then
  echo "dead Docker observer published a valid summary" >&2
  exit 1
fi

status=0
FAKE_DOCKER_MODE=fail-mid-lane CI_CAPTURE_DOCKER_EVENTS=testcontainers CI_REPORT_DIR="$test_dir" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" docker-events-fail-mid-lane 5s bash -c 'sleep 3' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane whose Docker observer died returned $status, want 2" >&2
  exit 1
fi
if [[ -e $test_dir/docker-events-fail-mid-lane-docker-summary.json ]]; then
  echo "failed Docker observer published a valid summary" >&2
  exit 1
fi

status=0
CI_CAPTURE_RESOURCES=1 CI_MAX_RSS_MB=0 CI_REPORT_DIR="$test_dir" \
  "$repo_root/scripts/run-ci-lane.sh" resource-budget 5s bash -c 'sleep 1' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane over its RSS budget returned $status, want 2" >&2
  exit 1
fi
grep -q '"budget_status":"exceeded"' "$test_dir/resource-budget-resources.json"

cat > "$test_dir/failing-resource-observer" <<'EOF'
#!/usr/bin/env bash
exit 9
EOF
chmod +x "$test_dir/failing-resource-observer"
status=0
CI_CAPTURE_RESOURCES=1 CI_RESOURCE_OBSERVER="$test_dir/failing-resource-observer" \
  CI_REPORT_DIR="$test_dir" \
  "$repo_root/scripts/run-ci-lane.sh" resource-observer-failure 5s bash -c 'sleep 1' || status=$?
if [[ $status -ne 2 ]]; then
  echo "lane whose resource observer died returned $status, want 2" >&2
  exit 1
fi
if [[ -e $test_dir/resource-observer-failure-resources.json ]]; then
  echo "failed resource observer published a valid summary" >&2
  exit 1
fi

echo "run-ci-lane tests passed"
