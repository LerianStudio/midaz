#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <downloaded-integration-artifacts>" >&2
  exit 2
fi

artifact_root=$1
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
allowlist=${INTEGRATION_SKIP_ALLOWLIST:-$repo_root/ci/integration-skip-allowlist.json}
capability='integration-chaos:CHAOS=1'

if [[ ! -d $artifact_root || ! -r $allowlist ]]; then
  echo "integration artifacts and skip allowlist must be readable" >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required to verify integration capability artifacts" >&2
  exit 2
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/midaz-capability-proof.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT

expected=$work_dir/expected.tsv
jq -r --arg capability "$capability" \
  '.entries[] | select(.alternate_capability == $capability) | [.package, .test] | @tsv' \
  "$allowlist" | sort > "$expected"
if [[ ! -s $expected ]]; then
  echo "chaos capability allowlist selected zero identities" >&2
  exit 1
fi
duplicates=$(uniq -d "$expected")
if [[ -n $duplicates ]]; then
  echo "chaos capability allowlist contains duplicate identity: ${duplicates%%$'\n'*}" >&2
  exit 1
fi

# Match only the per-shard aggregate outcomes.tsv. The wildcard in -path
# crosses directory separators, so the per-job copies under
# integration-shards/<shard>/jobs/<n>/outcomes.tsv must be excluded.
mapfile -t outcome_files < <(find "$artifact_root" -type f -path '*/integration-shards/*/outcomes.tsv' ! -path '*/integration-shards/*/jobs/*' | sort)
if [[ ${#outcome_files[@]} -ne 6 ]]; then
  echo "required integration proof has ${#outcome_files[@]} outcome artifacts, want 6" >&2
  exit 1
fi

base_skipped=$work_dir/base-skipped.tsv
capability_passed=$work_dir/capability-passed.tsv
: > "$base_skipped"
: > "$capability_passed"
capability_outcomes=
for outcomes in "${outcome_files[@]}"; do
  shard=$(basename "$(dirname "$outcomes")")
  if [[ $shard == chaos-capability ]]; then
    if [[ -n $capability_outcomes ]]; then
      echo "required integration proof contains duplicate chaos capability artifacts" >&2
      exit 1
    fi
    capability_outcomes=$outcomes
    awk -F '\t' 'NR > 1 {
      if ($3 != "passed") {
        printf "chaos capability %s#%s ended as %s, want passed\n", $1, $2, $3 > "/dev/stderr"
        bad=1
      }
      print $1 "\t" $2
    } END { exit bad }' "$outcomes" | sort > "$capability_passed"
    continue
  fi

  awk -F '\t' -v capability="$capability" 'NR > 1 && $3 == "skipped" {
    if ($5 != capability) {
      printf "base skip %s#%s names capability %s, want %s\n", $1, $2, $5, capability > "/dev/stderr"
      bad=1
    }
    print $1 "\t" $2
  } END { exit bad }' "$outcomes" >> "$base_skipped"
done

if [[ -z $capability_outcomes ]]; then
  echo "required integration proof is missing chaos capability outcomes" >&2
  exit 1
fi
sort -o "$base_skipped" "$base_skipped"

duplicates=$(uniq -d "$base_skipped")
if [[ -n $duplicates ]]; then
  echo "base integration artifacts contain duplicate skipped identity: ${duplicates%%$'\n'*}" >&2
  exit 1
fi
duplicates=$(uniq -d "$capability_passed")
if [[ -n $duplicates ]]; then
  echo "chaos capability artifact contains duplicate passed identity: ${duplicates%%$'\n'*}" >&2
  exit 1
fi
if ! diff -u "$expected" "$base_skipped"; then
  echo "base integration skips do not exactly match the versioned capability allowlist" >&2
  exit 1
fi
if ! diff -u "$expected" "$capability_passed"; then
  echo "chaos capability passes do not exactly match the versioned capability allowlist" >&2
  exit 1
fi

summary=$(dirname "$capability_outcomes")/summary.json
expected_count=$(wc -l < "$expected" | tr -d ' ')
jq -e --argjson count "$expected_count" '
  .shard == "chaos-capability" and
  .selected_test_count == $count and
  .passed_test_count == $count and
  .covered_test_count == $count and
  .skipped_test_count == 0 and
  .failed_test_count == 0 and
  .missing_test_count == 0 and
  .unclassified_test_count == 0 and
  .uncovered_test_count == 0 and
  .flake_budget == 0
' "$summary" >/dev/null

printf 'required integration capability proved %d exact package#test identities\n' "$expected_count"
