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

mkdir -p "$test_dir/bin"
cat > "$test_dir/bin/docker" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${FAKE_DOCKER_ARGS_FILE:-/dev/null}"
if [[ ${FAKE_DOCKER_MODE:-} == fail-start ]]; then
  exit 9
fi
printf '%s\n' \
  '{"Type":"container","Action":"start"}' \
  '{"Type":"container","Action":"start"}' \
  '{"Type":"container","Action":"restart"}'
if [[ ${FAKE_DOCKER_MODE:-} == fail-mid-lane ]]; then
  sleep 2
  exit 9
fi
exec sleep 30
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
  CI_CAPTURE_RESOURCES=1 CI_REPORT_DIR="$test_dir" \
  FAKE_DOCKER_ARGS_FILE="$test_dir/owner-docker-args.txt" \
  PATH="$test_dir/bin:$PATH" \
  "$repo_root/scripts/run-ci-lane.sh" owner-events 5s bash -c \
    'test "$TESTCONTAINERS_SESSION_ID" = midaz-owner-test; sleep 1'
grep -q 'label=org.testcontainers.sessionId=midaz-owner-test' "$test_dir/owner-docker-args.txt"
grep -q '"owner":"midaz-owner-test"' "$test_dir/owner-events-docker-summary.json"
grep -q '"peak_live_containers":2' "$test_dir/owner-events-docker-summary.json"
grep -q '"peak_rss_mb":' "$test_dir/owner-events-resources.json"
grep -q '"average_cpu_percent":' "$test_dir/owner-events-resources.json"

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
