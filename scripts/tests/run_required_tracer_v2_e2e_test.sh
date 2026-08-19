#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
mkdir -p "$test_dir/bin"

auth_configurator="$repo_root/scripts/configure-required-tracer-e2e-auth.sh"
if [[ ! -x $auth_configurator ]]; then
  echo "required Tracer E2E auth configurator is missing or not executable" >&2
  exit 1
fi

# sed_inplace edits a file in place portably: BSD sed (macOS) requires an
# explicit suffix argument after -i, GNU sed treats a separate suffix as a
# filename. Rewrite via a temp file instead of feature-detecting sed.
sed_inplace() {
  local expression=$1
  local file=$2
  local tmp
  tmp=$(mktemp)
  sed "$expression" "$file" > "$tmp"
  mv "$tmp" "$file"
}

read_env_value() {
  local env_file=$1
  local key=$2
  local line value= count=0
  while IFS= read -r line || [[ -n $line ]]; do
    case $line in
      "$key="*)
        value=${line#*=}
        count=$((count + 1))
        ;;
    esac
  done < "$env_file"
  [[ $count -eq 1 ]] || return 1
  printf '%s' "$value"
}

mkdir -p "$test_dir/clean" "$test_dir/second" "$test_dir/empty" "$test_dir/mismatch"
for fixture in clean second empty mismatch; do
  cp "$repo_root/components/ledger/.env.example" "$test_dir/$fixture/ledger.env"
  cp "$repo_root/components/tracer/.env.example" "$test_dir/$fixture/tracer.env"
  sed_inplace 's/^TRACER_TRANSPORT=.*/TRACER_TRANSPORT=rest/' "$test_dir/$fixture/ledger.env"
done

config_output=$(LEDGER_ENV_FILE="$test_dir/clean/ledger.env" TRACER_ENV_FILE="$test_dir/clean/tracer.env" \
  "$auth_configurator" 2>&1)
ledger_key=$(read_env_value "$test_dir/clean/ledger.env" TRACER_API_KEY)
tracer_key=$(read_env_value "$test_dir/clean/tracer.env" API_KEY)
[[ $ledger_key == "$tracer_key" ]]
[[ $ledger_key =~ ^[0-9a-f]{64}$ ]]
[[ $(read_env_value "$test_dir/clean/tracer.env" API_KEY_ENABLED) == true ]]
[[ $config_output != *"$ledger_key"* ]]

LEDGER_ENV_FILE="$test_dir/second/ledger.env" TRACER_ENV_FILE="$test_dir/second/tracer.env" \
  "$auth_configurator" >/dev/null
second_key=$(read_env_value "$test_dir/second/ledger.env" TRACER_API_KEY)
[[ $second_key != "$ledger_key" ]]

cat > "$test_dir/bin/require-e2e-key" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ ${E2E_TRACER_API_KEY:-} =~ ^[0-9a-f]{64}$ ]]
printf '%s\n' started > "$E2E_AUTH_CHILD_MARKER"
EOF
chmod +x "$test_dir/bin/require-e2e-key"

auth_child_marker="$test_dir/auth-child-started"
E2E_AUTH_CHILD_MARKER="$auth_child_marker" \
  LEDGER_ENV_FILE="$test_dir/clean/ledger.env" TRACER_ENV_FILE="$test_dir/clean/tracer.env" \
  "$auth_configurator" --exec "$test_dir/bin/require-e2e-key"
test -s "$auth_child_marker"
rm -f "$auth_child_marker"

sed_inplace 's/^API_KEY=.*/API_KEY=/' "$test_dir/empty/tracer.env"
status=0
E2E_AUTH_CHILD_MARKER="$auth_child_marker" \
  LEDGER_ENV_FILE="$test_dir/empty/ledger.env" TRACER_ENV_FILE="$test_dir/empty/tracer.env" \
  "$auth_configurator" --exec "$test_dir/bin/require-e2e-key" >/dev/null 2>&1 || status=$?
if [[ $status -eq 0 ]]; then
  echo "required Tracer E2E auth accepted empty Ledger and Tracer keys" >&2
  exit 1
fi
if [[ -e $auth_child_marker ]]; then
  echo "required Tracer E2E auth started the test process with empty keys" >&2
  exit 1
fi

sed_inplace 's/^TRACER_API_KEY=.*/TRACER_API_KEY=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/' "$test_dir/mismatch/ledger.env"
sed_inplace 's/^API_KEY=.*/API_KEY=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/' "$test_dir/mismatch/tracer.env"
sed_inplace 's/^API_KEY_ENABLED=.*/API_KEY_ENABLED=true/' "$test_dir/mismatch/tracer.env"
status=0
E2E_AUTH_CHILD_MARKER="$auth_child_marker" \
  LEDGER_ENV_FILE="$test_dir/mismatch/ledger.env" TRACER_ENV_FILE="$test_dir/mismatch/tracer.env" \
  "$auth_configurator" --exec "$test_dir/bin/require-e2e-key" >/dev/null 2>&1 || status=$?
if [[ $status -eq 0 ]]; then
  echo "required Tracer E2E auth accepted mismatched Ledger and Tracer keys" >&2
  exit 1
fi
if [[ -e $auth_child_marker ]]; then
  echo "required Tracer E2E auth started the test process with mismatched keys" >&2
  exit 1
fi

grep -Fq 'scripts/configure-required-tracer-e2e-auth.sh' "$repo_root/.github/workflows/pr-validation.yml"

cat > "$test_dir/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case ${FAKE_LEDGER_MODE:-legacy} in
  v2)
    printf '%s\n' '{"status":"healthy","tracer_outcome_mode":"ledger_outcome_v2"}'
    ;;
  legacy)
    printf '%s\n' '{"status":"healthy","tracer_outcome_mode":"legacy"}'
    ;;
  unavailable)
    exit 7
    ;;
esac
EOF
chmod +x "$test_dir/bin/curl"

cat > "$test_dir/bin/gotestsum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
json_file=
junit_file=
while [[ $# -gt 0 ]]; do
  case $1 in
    --jsonfile) json_file=$2; shift 2 ;;
    --junitfile) junit_file=$2; shift 2 ;;
    *) shift ;;
  esac
done
printf '%s\n' called > "$FAKE_GOTESTSUM_MARKER"
printf '%s\n' '{"Action":"pass","Test":"TestTracerOutcomeV2LedgerToTracerAndLostACK"}' > "$json_file"
printf '%s\n' '<testsuite name="fake"/>' > "$junit_file"
EOF
chmod +x "$test_dir/bin/gotestsum"

marker="$test_dir/gotestsum-called"
status=0
PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/legacy" FAKE_LEDGER_MODE=legacy \
  FAKE_GOTESTSUM_MARKER="$marker" E2E_TRACER_V2_WALL_TIMEOUT=5s \
  "$repo_root/scripts/run-required-tracer-v2-e2e.sh" || status=$?
if [[ $status -eq 0 ]]; then
  echo "Tracer V2 E2E accepted a running Ledger in legacy outcome mode" >&2
  exit 1
fi
if [[ -e $marker ]]; then
  echo "Tracer V2 E2E started tests before rejecting legacy Ledger mode" >&2
  exit 1
fi
grep -q '"status":"failed"' "$test_dir/legacy/ledger-tracer-v2-e2e-timing.json"

PATH="$test_dir/bin:$PATH" CI_REPORT_DIR="$test_dir/v2" FAKE_LEDGER_MODE=v2 \
  FAKE_GOTESTSUM_MARKER="$marker" E2E_TRACER_V2_WALL_TIMEOUT=5s \
  "$repo_root/scripts/run-required-tracer-v2-e2e.sh"
test -s "$marker"
grep -q '"status":"passed"' "$test_dir/v2/ledger-tracer-v2-e2e-timing.json"

echo "run-required-tracer-v2-e2e tests passed"
