#!/usr/bin/env bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
discover="$repo_root/scripts/discover-tagged-test-packages.sh"
list_tests="$repo_root/scripts/list-selected-go-tests.sh"
list_tagged_functions="$repo_root/scripts/list-tagged-test-functions.sh"
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
[[ -x "$merge_coverprofiles" ]] || fail "Go coverprofile merge helper is missing or not executable"

for portable_script in "$discover" "$list_tests" "$list_tagged_functions" "$merge_coverprofiles"; do
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

filtered_root_recipe=$(make -s -C "$repo_root" -n test-integration RUN=TestDifferentName)
assert_contains "$filtered_root_recipe" 'requested_pattern="TestDifferentName"' "root explicit RUN intersection"
assert_contains "$filtered_root_recipe" '"integration" "$top_level_pattern"' "root explicit RUN intersection"
assert_contains "$filtered_root_recipe" '-run "$exact_pattern"' "root exact tagged-test filter"

subtest_root_recipe=$(make -s -C "$repo_root" -n test-integration RUN=TestDifferentName/subcase)
assert_contains "$subtest_root_recipe" 'run_suffix="/${requested_pattern#*/}"' "root explicit subtest RUN suffix"
assert_contains "$subtest_root_recipe" 'exact_pattern="^($test_alternation)${test_anchor}${run_suffix}"' "root exact subtest filter"

default_tracer_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration)
assert_not_contains "$default_tracer_recipe" "-run '^TestIntegration'" "tracer default integration recipe"
assert_contains "$default_tracer_recipe" "./internal/... ./pkg/... ./tests/integration/..." "tracer integration scope"
assert_not_contains "$default_tracer_recipe" "./components/..." "tracer integration scope"

filtered_tracer_recipe=$(make -s -C "$repo_root/components/tracer" -n test-integration RUN=TestDifferentName)
assert_contains "$filtered_tracer_recipe" 'requested_pattern="TestDifferentName"' "tracer explicit RUN intersection"
assert_contains "$filtered_tracer_recipe" '"integration,testhooks" "$top_level_pattern"' "tracer explicit RUN intersection"
assert_contains "$filtered_tracer_recipe" '-run "$exact_pattern"' "tracer exact tagged-test filter"

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

mkdir -p "$fixture/selected" "$fixture/negated" "$fixture/empty" "$fixture/ordinary"

cat >"$fixture/go.mod" <<'EOF'
module example.com/gates

go 1.26
EOF

for package_name in selected negated empty ordinary; do
  cat >"$fixture/$package_name/package.go" <<EOF
package $package_name
EOF
done

cat >"$fixture/selected/tagged_test.go" <<'EOF'
//go:build integration || chaos

package selected

import "testing"

func TestDifferentName(t *testing.T) {}
EOF

cat >"$fixture/selected/ordinary_test.go" <<'EOF'
package selected

import "testing"

func TestOrdinary(t *testing.T) {}
EOF

cat >"$fixture/negated/negated_test.go" <<'EOF'
//go:build !integration

package negated

import "testing"

func TestExcluded(t *testing.T) {}
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

selected_packages=$(cd "$fixture" && "$discover" integration integration ./selected)
assert_contains "$selected_packages" "example.com/gates/selected" "compound integration build tag"

if (cd "$fixture" && "$discover" integration integration ./negated >negated.out 2>negated.err); then
  fail "a negated integration tag was discovered as selected"
fi
assert_contains "$(cat "$fixture/negated.err")" "no Go packages contain tests selected by build tag \"integration\"" "negated tag diagnostic"

if (cd "$fixture" && "$discover" integration integration ./ordinary >ordinary.out 2>ordinary.err); then
  fail "an untagged package was discovered as integration"
fi
assert_contains "$(cat "$fixture/ordinary.err")" "no Go packages contain tests selected by build tag \"integration\"" "zero-package diagnostic"

if (cd "$fixture" && "$discover" integration integration ./missing >missing.out 2>missing.err); then
  fail "go list failure was swallowed"
fi
[[ -s "$fixture/missing.err" ]] || fail "go list failure produced no stderr diagnostic"

all_selected_tests=$(cd "$fixture" && "$list_tests" integration . example.com/gates/selected)
assert_contains "$all_selected_tests" "example.com/gates/selected TestDifferentName" "tagged integration test listing"
assert_contains "$all_selected_tests" "example.com/gates/selected TestOrdinary" "co-located ordinary test listing"

tagged_functions=$(cd "$fixture" && "$list_tagged_functions" integration integration ./selected)
assert_contains "$tagged_functions" "example.com/gates/selected TestDifferentName" "tag-selected test function listing"
assert_not_contains "$tagged_functions" "TestOrdinary" "tag-selected test function listing"

filtered_tests=$(cd "$fixture" && "$list_tests" integration TestDifferentName example.com/gates/selected)
assert_contains "$filtered_tests" "example.com/gates/selected TestDifferentName" "explicit RUN listing"
assert_not_contains "$filtered_tests" "TestOrdinary" "explicit RUN listing"

if (cd "$fixture" && "$list_tests" integration DoesNotExist example.com/gates/selected >run.out 2>run.err); then
  fail "an explicit RUN matching zero tests passed"
fi
assert_contains "$(cat "$fixture/run.err")" "selected zero runnable tests" "zero-test RUN diagnostic"

empty_packages=$(cd "$fixture" && "$discover" integration integration ./empty)
if (cd "$fixture" && "$list_tests" integration . $empty_packages >empty.out 2>empty.err); then
  fail "a tagged package containing zero runnable tests passed"
fi
assert_contains "$(cat "$fixture/empty.err")" "selected zero runnable tests" "zero-test package diagnostic"

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
