#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
discover="$repo_root/scripts/discover-tagged-test-packages.sh"
list_tests="$repo_root/scripts/list-selected-go-tests.sh"
list_tagged_functions="$repo_root/scripts/list-tagged-test-functions.sh"
verify_events="$repo_root/scripts/verify-selected-go-test-events.sh"
merge_coverprofiles="$repo_root/scripts/merge-go-coverprofiles.sh"

fail() {
  echo "[error] $*" >&2
  exit 1
}

assert_contains() {
  local haystack=$1
  local needle=$2
  local context=$3

  [[ "$haystack" == *"$needle"* ]] || fail "$context: missing '$needle'"
}

assert_not_contains() {
  local haystack=$1
  local needle=$2
  local context=$3

  [[ "$haystack" != *"$needle"* ]] || fail "$context: unexpectedly contained '$needle'"
}

[[ -x "$discover" ]] || fail "tagged-package discovery helper is missing or not executable"
[[ -x "$list_tests" ]] || fail "selected-test listing helper is missing or not executable"
[[ -x "$list_tagged_functions" ]] || fail "tagged test-function listing helper is missing or not executable"
[[ -x "$verify_events" ]] || fail "selected-test event verifier is missing or not executable"
[[ -x "$merge_coverprofiles" ]] || fail "Go coverprofile merge helper is missing or not executable"

for portable_script in "$discover" "$list_tests" "$list_tagged_functions" "$verify_events" "$merge_coverprofiles"; do
  if grep -Fq 'declare -A' "$portable_script"; then
    fail "helper requires associative arrays unavailable in macOS Bash 3.2: ${portable_script#"$repo_root/"}"
  fi
  if grep -Fq '\<' "$portable_script" || grep -Fq '\>' "$portable_script"; then
    fail "helper uses GNU-only grep word boundaries: ${portable_script#"$repo_root/"}"
  fi
done

default_root_recipe=$(make -s -C "$repo_root" -n test-integration)
assert_not_contains "$default_root_recipe" "-run '^TestIntegration'" "root default integration recipe"
assert_contains "$default_root_recipe" "./components/... ./pkg/... ./tests/..." "root integration scope"

reported_root_recipe=$(make -s -C "$repo_root" -n test-integration \
  GOTESTSUM=gotestsum TEST_REPORTS_DIR=reports/ci)
assert_contains "$reported_root_recipe" 'reports/ci/integration-events/$package_index.json' \
  "root per-package integration event report"
assert_contains "$reported_root_recipe" 'gotestsum $gotestsum_event_flag --format testname' \
  "root machine-readable integration event report"

filtered_root_recipe=$(make -s -C "$repo_root" -n test-integration RUN=TestDifferentName)
assert_not_contains "$filtered_root_recipe" 'requested_pattern="TestDifferentName"' "root RUN must not be interpolated into shell"
assert_contains "$filtered_root_recipe" 'MIDAZ_TEST_RUN' "root RUN environment transport"
assert_contains "$filtered_root_recipe" 'printf '\''%s\n'\'' "$tagged_tests" | "' "root static RUN intersection"
assert_contains "$filtered_root_recipe" 'pkgs=$(printf '\''%s\n'\'' "$selected_tests"' "root RUN package recalculation"
assert_contains "$filtered_root_recipe" '-run "$exact_pattern"' "root exact tagged-test filter"

subtest_root_recipe=$(make -s -C "$repo_root" -n test-integration RUN=TestDifferentName/subcase)
assert_not_contains "$subtest_root_recipe" 'run_suffix=' "root shell must not parse RUN"
assert_contains "$subtest_root_recipe" 'run-patterns.tsv' "root Go-derived runtime pattern"
assert_contains "$subtest_root_recipe" 'verify-selected-go-test-events.sh' "root subtest execution verification"

subtest_coverage_recipe=$(make -s -C "$repo_root" -n coverage-integration RUN=TestDifferentName/subcase)
assert_contains "$subtest_coverage_recipe" 'pkgs=$(printf '\''%s\n'\'' "$selected_tests"' "coverage RUN package recalculation"
assert_contains "$subtest_coverage_recipe" 'verify-selected-go-test-events.sh' "coverage subtest execution verification"

default_tracer_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration)
assert_not_contains "$default_tracer_recipe" "-run '^TestIntegration'" "tracer default integration recipe"
assert_contains "$default_tracer_recipe" "./internal/... ./pkg/... ./tests/integration/..." "tracer integration scope"
assert_not_contains "$default_tracer_recipe" "./components/..." "tracer integration scope"

filtered_tracer_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration RUN=TestDifferentName)
assert_not_contains "$filtered_tracer_recipe" 'requested_pattern="TestDifferentName"' "tracer RUN must not be interpolated into shell"
assert_contains "$filtered_tracer_recipe" 'MIDAZ_TEST_RUN' "tracer RUN environment transport"
assert_contains "$filtered_tracer_recipe" 'printf '\''%s\n'\'' "$tagged_tests" | "' "tracer static RUN intersection"
assert_contains "$filtered_tracer_recipe" 'pkgs=$(printf '\''%s\n'\'' "$selected_tests"' "tracer RUN package recalculation"
assert_contains "$filtered_tracer_recipe" '-run "$exact_pattern"' "tracer exact tagged-test filter"

tracer_coverage_recipe=$(make -s -C "$repo_root/components/tracer" -n coverage-integration RUN=TestDifferentName/subcase)
if ! printf '%s\n' "$tracer_coverage_recipe" | /bin/sh -n; then
  fail "tracer coverage integration recipe is not valid POSIX shell"
fi

streaming_alternatives='TestStreamingSmoke/missing|TestStreamingSmoke'
streaming_test_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration RUN="$streaming_alternatives")
assert_contains "$streaming_test_recipe" 'verify-selected-go-test-events.sh' "test alternative event verification"
streaming_coverage_recipe=$(make -s -C "$repo_root/components/tracer" -n coverage-integration RUN="$streaming_alternatives")
assert_contains "$streaming_coverage_recipe" 'verify-selected-go-test-events.sh' "coverage alternative event verification"
streaming_fallback_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration \
  RUN="$streaming_alternatives" GOTESTSUM=)
assert_contains "$streaming_fallback_recipe" 'go test -json' "fallback alternative JSON capture"
assert_contains "$streaming_fallback_recipe" 'verify-selected-go-test-events.sh' "fallback alternative event verification"

property_recipe=$(make -s -C "$repo_root" -n test-property)
assert_contains "$property_recipe" "./components/... ./pkg/... ./tests/..." "property scope"
assert_not_contains "$property_recipe" "command -v docker" "property recipe must stay Docker-free"
assert_contains "$property_recipe" 'exact_pattern="^(' "property exact per-package filter"
assert_contains "$property_recipe" '-run "$exact_pattern"' "property exact per-package filter"

property_file_count=0
while IFS= read -r property_file; do
  property_file_count=$((property_file_count + 1))
  grep -qx '//go:build property' "$property_file" \
    || fail "property test lacks the property build tag: ${property_file#"$repo_root/"}"
done < <(find "$repo_root/components" "$repo_root/pkg" "$repo_root/tests" \
  -type f -name '*_property_test.go' -print | sort)

((property_file_count >= 16)) \
  || fail "expected at least the 16 existing property test files, found $property_file_count"

fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT

mkdir -p \
  "$fixture/selected" \
  "$fixture/other" \
  "$fixture/compound" \
  "$fixture/negated" \
  "$fixture/tautology" \
  "$fixture/testhooks_only" \
  "$fixture/empty" \
  "$fixture/ordinary" \
  "$fixture/bin"

cat >"$fixture/go.mod" <<'EOF'
module example.com/gates

go 1.26
EOF

for package_name in selected other compound negated tautology testhooks_only empty ordinary; do
  cat >"$fixture/$package_name/package.go" <<EOF
package $package_name
EOF
done

cat >"$fixture/selected/tagged_test.go" <<'EOF'
//go:build integration || chaos

package selected

import (
  "os"
  "os/exec"
  "testing"
)

func TestMain(m *testing.M) {
  if sentinel := os.Getenv("TESTMAIN_SENTINEL"); sentinel != "" {
    _ = os.WriteFile(sentinel, []byte("executed"), 0o600)
  }
  _ = exec.Command("docker", "version").Run()
  os.Exit(m.Run())
}

func TestDifferentName(t *testing.T) {
  t.Run("present", func(t *testing.T) {})
}

func TestA(t *testing.T) {
  t.Run("B", func(t *testing.T) {
    t.Run("leaf", func(t *testing.T) {})
  })
}

func TestC(t *testing.T) {
  t.Run("D", func(t *testing.T) {
    t.Run("leaf", func(t *testing.T) {})
  })
}

func TestReservationMTLS(t *testing.T) {}
EOF

cat >"$fixture/selected/ordinary_test.go" <<'EOF'
package selected

import "testing"

func TestOrdinary(t *testing.T) {}
EOF

cat >"$fixture/other/tagged_test.go" <<'EOF'
//go:build integration

package other

import "testing"

func TestOther(t *testing.T) {}
EOF

cat >"$fixture/compound/tagged_test.go" <<'EOF'
//go:build integration && linux

package compound

import "testing"

func TestCompound(t *testing.T) {}
EOF

cat >"$fixture/negated/negated_test.go" <<'EOF'
//go:build !integration

package negated

import "testing"

func TestExcluded(t *testing.T) {}
EOF

cat >"$fixture/tautology/tagged_test.go" <<'EOF'
//go:build integration || !integration

package tautology

import "testing"

func TestNotOwnedByIntegration(t *testing.T) {}
EOF

cat >"$fixture/testhooks_only/tagged_test.go" <<'EOF'
//go:build integration || testhooks

package testhooks_only

import "testing"

func TestOwnedByTesthooks(t *testing.T) {}
EOF

cat >"$fixture/empty/tagged_test.go" <<'EOF'
//go:build integration

package empty

func taggedHelper() {}
EOF

cat >"$fixture/ordinary/ordinary_test.go" <<'EOF'
package ordinary

import "testing"

func TestOrdinary(t *testing.T) {}
EOF

cat >"$fixture/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: >"$DOCKER_SENTINEL"
EOF
chmod +x "$fixture/bin/docker"

selected_packages=$(cd "$fixture" && "$discover" integration integration ./selected)
assert_contains "$selected_packages" "example.com/gates/selected" "compound integration build tag"

compound_packages=$(cd "$fixture" && "$discover" integration integration ./compound)
assert_contains "$compound_packages" "example.com/gates/compound" "conjunctive integration build tag"

if (cd "$fixture" && "$discover" integration integration ./negated >negated.out 2>negated.err); then
  fail "a negated integration tag was discovered as selected"
fi
assert_contains "$(cat "$fixture/negated.err")" "no Go packages contain tests selected by build tag \"integration\"" "negated tag diagnostic"

if (cd "$fixture" && "$discover" integration integration ./tautology >tautology.out 2>tautology.err); then
  fail "a constraint true with and without integration was assigned to integration"
fi
assert_contains "$(cat "$fixture/tautology.err")" "no Go packages contain tests selected by build tag \"integration\"" "tautological constraint diagnostic"

if (cd "$fixture" && "$discover" integration integration,testhooks ./testhooks_only >testhooks.out 2>testhooks.err); then
  fail "a testhooks-selected file was assigned to integration"
fi
assert_contains "$(cat "$fixture/testhooks.err")" "no Go packages contain tests selected by build tag \"integration\"" "compound alternate-tag diagnostic"

if (cd "$fixture" && "$discover" integration integration ./ordinary >ordinary.out 2>ordinary.err); then
  fail "an untagged package was discovered as integration"
fi
assert_contains "$(cat "$fixture/ordinary.err")" "no Go packages contain tests selected by build tag \"integration\"" "zero-package diagnostic"

if (cd "$fixture" && "$discover" integration integration ./missing >missing.out 2>missing.err); then
  fail "go list failure was swallowed"
fi
[[ -s "$fixture/missing.err" ]] || fail "go list failure produced no stderr diagnostic"

tagged_functions=$(cd "$fixture" && "$list_tagged_functions" integration integration ./selected ./other)
assert_contains "$tagged_functions" "example.com/gates/selected TestDifferentName" "tag-selected test function listing"
assert_contains "$tagged_functions" "example.com/gates/other TestOther" "second tag-selected package listing"
assert_not_contains "$tagged_functions" "TestOrdinary" "tag-selected test function listing"

testmain_sentinel="$fixture/testmain.executed"
docker_sentinel="$fixture/docker.executed"
filtered_tests=$(cd "$fixture" && \
  printf '%s\n' "$tagged_functions" \
    | PATH="$fixture/bin:$PATH" TESTMAIN_SENTINEL="$testmain_sentinel" DOCKER_SENTINEL="$docker_sentinel" \
      "$list_tests" TestDifferentName "$fixture/filtered-patterns.tsv")
assert_contains "$filtered_tests" "example.com/gates/selected TestDifferentName" "explicit RUN listing"
assert_not_contains "$filtered_tests" "example.com/gates/other" "explicit RUN package narrowing"
assert_contains "$(cat "$fixture/filtered-patterns.tsv")" 'example.com/gates/selected' "runtime pattern package"
[[ ! -e "$testmain_sentinel" ]] || fail "static RUN selection executed TestMain"
[[ ! -e "$docker_sentinel" ]] || fail "static RUN selection executed Docker"

if (cd "$fixture" && printf '%s\n' "$tagged_functions" \
  | "$list_tests" DoesNotExist "$fixture/missing-patterns.tsv" >run.out 2>run.err); then
  fail "an explicit RUN matching zero tests passed"
fi
assert_contains "$(cat "$fixture/run.err")" "selected zero runnable tests" "zero-test RUN diagnostic"

selected_inventory=$(cd "$fixture" && "$list_tagged_functions" integration integration ./selected)

class_tests=$(printf '%s\n' "$selected_inventory" \
  | "$list_tests" 'TestReservationMTL[S/]' "$fixture/class-patterns.tsv")
assert_contains "$class_tests" "example.com/gates/selected TestReservationMTLS" "slash inside character class"
assert_not_contains "$class_tests" "TestA" "slash inside character class"

alternative_tests=$(printf '%s\n' "$selected_inventory" \
  | "$list_tests" 'A/B|C/D' "$fixture/alternative-patterns.tsv")
assert_contains "$alternative_tests" "example.com/gates/selected TestA" "path alternative A/B"
assert_contains "$alternative_tests" "example.com/gates/selected TestC" "path alternative C/D"
assert_contains "$(cat "$fixture/alternative-patterns.tsv")" '^(TestA)$/B|^(TestC)$/D' "runtime path alternatives"

escaped_slash_tests=$(printf '%s\n' "$selected_inventory" \
  | "$list_tests" 'A\/B|C/D' "$fixture/escaped-slash-patterns.tsv")
assert_not_contains "$escaped_slash_tests" "TestA" "escaped slash must not split a path"
assert_contains "$escaped_slash_tests" "example.com/gates/selected TestC" "alternative after escaped slash"

grouped_slash_tests=$(printf '%s\n' "$selected_inventory" \
  | "$list_tests" '(A/B)|C/D' "$fixture/grouped-slash-patterns.tsv")
assert_not_contains "$grouped_slash_tests" "TestA" "slash inside a group must not split a path"
assert_contains "$grouped_slash_tests" "example.com/gates/selected TestC" "alternative after grouped slash"

multi_segment_tests=$(printf '%s\n' "$selected_inventory" \
  | "$list_tests" 'A/B/leaf|C/D/missing' "$fixture/multi-segment-patterns.tsv")
assert_contains "$multi_segment_tests" "example.com/gates/selected TestA" "multi-segment path A"
assert_contains "$multi_segment_tests" "example.com/gates/selected TestC" "multi-segment path C"

runtime_pattern_for() {
  awk -F '\t' '$1 == "example.com/gates/selected" { sub(/^[^\t]*\t/, ""); print }' "$1"
}

parity_index=0
assert_go_run_parity() {
  local raw_pattern=$1
  local runtime_pattern=$2
  local context=$3
  local raw_events
  local runtime_events
  local raw_names
  local runtime_names

  parity_index=$((parity_index + 1))
  raw_events="$fixture/parity-$parity_index-raw.json"
  runtime_events="$fixture/parity-$parity_index-runtime.json"

  (cd "$fixture" && \
    PATH="$fixture/bin:$PATH" TESTMAIN_SENTINEL="$testmain_sentinel" DOCKER_SENTINEL="$docker_sentinel" \
      go test -tags=integration -json -run "$raw_pattern" ./selected) >"$raw_events"
  (cd "$fixture" && \
    PATH="$fixture/bin:$PATH" TESTMAIN_SENTINEL="$testmain_sentinel" DOCKER_SENTINEL="$docker_sentinel" \
      go test -tags=integration -json -run "$runtime_pattern" ./selected) >"$runtime_events"

  raw_names=$(sed -n 's/.*"Action":"run".*"Test":"\([^"]*\)".*/\1/p' "$raw_events")
  runtime_names=$(sed -n 's/.*"Action":"run".*"Test":"\([^"]*\)".*/\1/p' "$runtime_events")
  [[ "$runtime_names" == "$raw_names" ]] \
    || fail "$context: generated RUN events differ from go test -run"
}

class_pattern=$(runtime_pattern_for "$fixture/class-patterns.tsv")
alternative_pattern=$(runtime_pattern_for "$fixture/alternative-patterns.tsv")
escaped_slash_pattern=$(runtime_pattern_for "$fixture/escaped-slash-patterns.tsv")
grouped_slash_pattern=$(runtime_pattern_for "$fixture/grouped-slash-patterns.tsv")
multi_segment_pattern=$(runtime_pattern_for "$fixture/multi-segment-patterns.tsv")

assert_go_run_parity 'TestReservationMTL[S/]' "$class_pattern" "character-class slash parity"
assert_go_run_parity 'A/B|C/D' "$alternative_pattern" "path-alternative parity"
assert_go_run_parity 'A\/B|C/D' "$escaped_slash_pattern" "escaped-slash parity"
assert_go_run_parity '(A/B)|C/D' "$grouped_slash_pattern" "grouped-slash parity"
assert_go_run_parity 'A/B/leaf|C/D/missing' "$multi_segment_pattern" "multi-segment parity"

streaming_inventory='example.com/gates/streaming TestStreamingSmoke'
for streaming_pattern in \
  'TestStreamingSmoke/missing|TestStreamingSmoke' \
  'TestStreamingSmoke|TestStreamingSmoke/missing'; do
  streaming_patterns_file="$fixture/streaming-$parity_index-patterns.tsv"
  printf '%s\n' "$streaming_inventory" \
    | "$list_tests" "$streaming_pattern" "$streaming_patterns_file" >/dev/null
  streaming_runtime_pattern=$(awk -F '\t' \
    '$1 == "example.com/gates/streaming" { sub(/^[^\t]*\t/, ""); print }' \
    "$streaming_patterns_file")
  printf '%s\n' '{"Action":"run","Test":"TestStreamingSmoke"}' \
    | "$verify_events" "$streaming_runtime_pattern"
done

if (cd "$fixture" && "$list_tagged_functions" integration integration ./empty >empty.out 2>empty.err); then
  fail "a tagged package containing zero runnable tests passed"
fi
assert_contains "$(cat "$fixture/empty.err")" "selected zero top-level Test functions" "zero-test package diagnostic"

(cd "$fixture" && printf '%s\n' "$selected_inventory" \
  | "$list_tests" 'DifferentName/missing' "$fixture/missing-subtest-patterns.tsv" >/dev/null)
missing_subtest_pattern=$(awk -F '\t' '$1 == "example.com/gates/selected" { sub(/^[^\t]*\t/, ""); print }' \
  "$fixture/missing-subtest-patterns.tsv")
(cd "$fixture" && \
  PATH="$fixture/bin:$PATH" TESTMAIN_SENTINEL="$testmain_sentinel" DOCKER_SENTINEL="$docker_sentinel" \
    go test -tags=integration -json -run "$missing_subtest_pattern" ./selected) \
    >"$fixture/missing-subtest.json"
if "$verify_events" "$missing_subtest_pattern" <"$fixture/missing-subtest.json" \
  >"$fixture/missing-subtest.out" 2>"$fixture/missing-subtest.err"; then
  fail "a RUN matching no started subtest passed"
fi
assert_contains "$(cat "$fixture/missing-subtest.err")" "started zero tests matching" "zero-subtest RUN diagnostic"

(cd "$fixture" && \
  PATH="$fixture/bin:$PATH" TESTMAIN_SENTINEL="$testmain_sentinel" DOCKER_SENTINEL="$docker_sentinel" \
    go test -tags=integration -json -run "$multi_segment_pattern" ./selected) \
    >"$fixture/present-subtest.json"
"$verify_events" "$multi_segment_pattern" <"$fixture/present-subtest.json"

dollar_sentinel="$fixture/dollar.executed"
dollar_pattern='TestReservationMTLS[$(touch '"$dollar_sentinel"')]*'
dollar_output=$(make -s -C "$repo_root/components/tracer" list-integration-tests \
  PKG=./internal/bootstrap RUN="$dollar_pattern")
assert_contains "$dollar_output" "TestReservationMTLS" "literal dollar command-substitution pattern"
[[ ! -e "$dollar_sentinel" ]] || fail 'RUN executed a literal $() payload'

backtick_sentinel="$fixture/backtick.executed"
backtick_pattern='TestReservationMTLS[`touch '"$backtick_sentinel"'`]*'
backtick_output=$(make -s -C "$repo_root/components/tracer" list-integration-tests \
  PKG=./internal/bootstrap RUN="$backtick_pattern")
assert_contains "$backtick_output" "TestReservationMTLS" "literal backtick pattern"
[[ ! -e "$backtick_sentinel" ]] || fail 'RUN executed a literal backtick payload'

quote_pattern="TestReservationMTLS[\"']*"
quote_output=$(make -s -C "$repo_root/components/tracer" list-integration-tests \
  PKG=./internal/bootstrap RUN="$quote_pattern")
assert_contains "$quote_output" "TestReservationMTLS" "literal quote pattern"

cat >"$fixture/first.cover" <<'EOF'
mode: atomic
example.com/gates/file.go:1.1,2.2 1 2
example.com/gates/file.go:4.1,5.2 2 1
EOF

cat >"$fixture/second.cover" <<'EOF'
mode: atomic
example.com/gates/file.go:1.1,2.2 1 3
example.com/gates/other.go:7.1,8.2 1 4
EOF

"$merge_coverprofiles" "$fixture/merged.cover" "$fixture/first.cover" "$fixture/second.cover"
assert_contains "$(cat "$fixture/merged.cover")" "example.com/gates/file.go:1.1,2.2 1 5" "merged coverprofile count"
assert_contains "$(cat "$fixture/merged.cover")" "example.com/gates/other.go:7.1,8.2 1 4" "merged coverprofile entry"

echo "[ok] Make test gate selection contract is honest"
