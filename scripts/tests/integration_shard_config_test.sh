#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
config="$repo_root/ci/integration-shards.tsv"

testcontainers_version=$(cd "$repo_root" && go list -m -f '{{.Version}}' github.com/testcontainers/testcontainers-go)
if [[ $testcontainers_version != v0.44.0 ]]; then
  echo "owner-aware Docker events require testcontainers-go v0.44.0, got $testcontainers_version" >&2
  exit 1
fi

[[ -s $config ]]
rows=$(awk -F '\t' '!/^#/ && NF { count++ } END { print count + 0 }' "$config")
if [[ $rows -ne 6 ]]; then
  echo "integration shard config has $rows rows, want 6" >&2
  exit 1
fi

expected_shards='async-broker
chaos-capability
ledger-mongodb-crm
ledger-postgres
lifecycle-migration
tracer'
actual_shards=$(awk -F '\t' '!/^#/ && NF { print $1 }' "$config" | sort)
if [[ $actual_shards != "$expected_shards" ]]; then
  echo "integration shard config does not contain the five stable shards and required chaos capability" >&2
  exit 1
fi

expected_config=$'ledger-postgres\t2\t2\t2\t400\t2048\t24\t0\t15m\t0\nledger-mongodb-crm\t2\t2\t2\t400\t2048\t16\t0\t15m\t0\nasync-broker\t2\t2\t2\t400\t3072\t20\t0\t15m\t0\ntracer\t2\t2\t2\t400\t2048\t6\t0\t15m\t1\nlifecycle-migration\t1\t1\t4\t400\t3072\t10\t0\t15m\t0\nchaos-capability\t3\t1\t2\t400\t2048\t10\t0\t15m\t0'
actual_config=$(awk '!/^#/ && NF' "$config")
if [[ $actual_config != "$expected_config" ]]; then
  echo "integration shard resource budgets changed without updating their contract" >&2
  exit 1
fi

awk -F '\t' '
  /^#/ || !NF { next }
  NF != 10 { printf "line %d has %d fields, want 10\n", NR, NF > "/dev/stderr"; exit 1 }
  $2 !~ /^[1-4]$/ || $3 !~ /^[1-4]$/ || $4 !~ /^[1-4]$/ { printf "line %d has an out-of-range parallelism field (want 1-4)\n", NR > "/dev/stderr"; exit 1 }
  $5 !~ /^[1-9][0-9]*$/ || $6 !~ /^[1-9][0-9]*$/ || $7 !~ /^[1-9][0-9]*$/ { printf "line %d has a non-positive-integer budget field\n", NR > "/dev/stderr"; exit 1 }
  $8 != 0 { print "flake budget must remain zero" > "/dev/stderr"; exit 1 }
  $9 !~ /^[0-9]+(m|h)$/ { print "invalid wall timeout" > "/dev/stderr"; exit 1 }
  $10 !~ /^[01]$/ { print "race flag must be 0 or 1" > "/dev/stderr"; exit 1 }
  $1 == "tracer" && $10 != 1 { print "tracer shard must run with race detection" > "/dev/stderr"; exit 1 }
  $1 != "tracer" && $10 != 0 { print "only tracer shard may enable race detection" > "/dev/stderr"; exit 1 }
  $1 == "lifecycle-migration" && ($2 != 1 || $3 != 1) { print "lifecycle shard is not serial" > "/dev/stderr"; exit 1 }
  $1 == "chaos-capability" && ($2 != 3 || $3 != 1) { print "chaos capability must use three isolated package workers" > "/dev/stderr"; exit 1 }
' "$config"

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
cat > "$test_dir/fake-lane" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" > "$FAKE_CAPTURE_DIR/args"
env | sort > "$FAKE_CAPTURE_DIR/env"
EOF
chmod +x "$test_dir/fake-lane"

FAKE_CAPTURE_DIR="$test_dir" \
  MIDAZ_CI_LANE_RUNNER="$test_dir/fake-lane" \
  GITHUB_RUN_ID=8123 GITHUB_RUN_ATTEMPT=2 \
  "$repo_root/scripts/run-integration-ci-shard.sh" ledger-postgres

grep -q '^CI_CAPTURE_DOCKER_EVENTS=owner$' "$test_dir/env"
grep -q '^CI_REQUIRE_DOCKER_OWNER_EVENTS=1$' "$test_dir/env"

grep -q '^CI_CAPTURE_RESOURCES=1$' "$test_dir/env"
grep -q '^CI_DOCKER_OWNER=midaz-8123-2-ledger-postgres$' "$test_dir/env"
grep -q '^CI_MAX_AVERAGE_CPU_PERCENT=400$' "$test_dir/env"
grep -q '^INTEGRATION_PACKAGE_PARALLELISM=2$' "$test_dir/env"
grep -q '^INTEGRATION_TEST_PARALLELISM=2$' "$test_dir/env"
grep -q '^INTEGRATION_JOB_GOMAXPROCS=2$' "$test_dir/env"
grep -q '^GOMAXPROCS=2$' "$test_dir/env"
grep -q '^INTEGRATION_FLAKE_BUDGET=0$' "$test_dir/env"
grep -q '^INTEGRATION_RACE=0$' "$test_dir/env"
if [[ $(nproc) -gt 4 && -n $(command -v taskset || true) ]]; then
  grep -q 'integration-ledger-postgres 15m taskset --cpu-list 0-3 make test-integration-shard INTEGRATION_SHARD=ledger-postgres' "$test_dir/args"
else
  grep -q 'integration-ledger-postgres 15m make test-integration-shard INTEGRATION_SHARD=ledger-postgres' "$test_dir/args"
fi

FAKE_CAPTURE_DIR="$test_dir" \
  MIDAZ_CI_LANE_RUNNER="$test_dir/fake-lane" \
  GITHUB_RUN_ID=8123 GITHUB_RUN_ATTEMPT=2 \
  "$repo_root/scripts/run-integration-ci-shard.sh" tracer
grep -q '^INTEGRATION_RACE=1$' "$test_dir/env"
grep -q '^CI_REQUIRE_DOCKER_OWNER_EVENTS=1$' "$test_dir/env"

FAKE_CAPTURE_DIR="$test_dir" \
  MIDAZ_CI_LANE_RUNNER="$test_dir/fake-lane" \
  GITHUB_RUN_ID=8123 GITHUB_RUN_ATTEMPT=2 \
  "$repo_root/scripts/run-integration-ci-shard.sh" chaos-capability
grep -q '^CI_MAX_RSS_MB=2048$' "$test_dir/env"
grep -q '^CI_MAX_PEAK_CONTAINERS=10$' "$test_dir/env"
grep -q '^INTEGRATION_PACKAGE_PARALLELISM=3$' "$test_dir/env"
grep -q '^INTEGRATION_TEST_PARALLELISM=1$' "$test_dir/env"
if [[ $(nproc) -gt 4 && -n $(command -v taskset || true) ]]; then
  grep -q 'integration-chaos-capability 15m taskset --cpu-list 20-23 make test-integration-shard INTEGRATION_SHARD=chaos-capability' "$test_dir/args"
else
  grep -q 'integration-chaos-capability 15m make test-integration-shard INTEGRATION_SHARD=chaos-capability' "$test_dir/args"
fi

echo "integration shard config tests passed"
