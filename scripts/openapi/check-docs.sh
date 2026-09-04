#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# OpenAPI documentation guardrail.
#
# (a) PARITY CHECK (always): asserts the native Huma OAS 3.1 dumps listed in
#     PARITY_DUMPS agree on .info.version and carry the ^4.0.0$ release, so the
#     published specs do not drift on the metadata the Huma dump actually emits.
#     A new dump must be added to that list to be covered. (Huma emits only
#     .info.title + .info.version; it does not populate
#     contact/license/termsOfService, and OAS 3.1 has no .schemes — those
#     swaggo-era parity fields are dropped honestly. See parity_check for why
#     .info.title is not asserted for byte parity.)
# (b) DRIFT CHECK (CHECK_DOCS_REGEN=1 only): regenerates the docs and asserts the
#     committed artifacts still reproduce, so the source annotations and the
#     committed specs cannot silently diverge.
# (c) SECURITY COVERAGE (always, ledger dumps only): asserts every ledger
#     operation carries a .security requirement, so the secure-by-default
#     contract cannot regress to a scheme declared but never required. Scoped to
#     ledger: tracer's /health, /readyz and /version are intentionally public.

# Root directory of the repo (this script lives in scripts/openapi/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

OPENAPI_DIR="${ROOT_DIR}/scripts/openapi"

# Huma OAS 3.1 dumps that must agree on shared metadata, as
# "<label>|<repo-relative path>". Each component publishes a single dump carrying its
# full surface, consolidated into api/midaz.openapi.{yaml,json}. The label is what
# failure messages name; the first entry is the parity reference.
PARITY_DUMPS=(
    "ledger|components/ledger/api/openapi.huma.yaml"
    "tracer|components/tracer/api/openapi.huma.yaml"
)

# Space-joined labels of the dump entries passed as arguments, for reporting which
# dumps a check covered.
dump_labels() {
    local entry labels=()

    for entry in "$@"; do
        labels+=("${entry%%|*}")
    done

    printf '%s' "${labels[*]}"
}

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  🔍 $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

# Diagnostics go to stderr: several checks read dump fields through command
# substitution, which would otherwise capture the message into a variable instead
# of showing it.
fail() {
    echo -e "    ${RED}❌ $1${NC}" >&2
    exit 1
}

ok() {
    echo -e "    ${GREEN}✅ $1${NC}" >&2
}

require_jq() {
    if ! command -v jq >/dev/null 2>&1; then
        fail "jq is required but was not found on PATH."
    fi
}

# Emit a Huma OAS 3.1 dump as JSON on stdout, given a "<label>|<path>" entry. jq
# cannot read YAML, so we convert via the same bundled js-yaml the generator uses
# for its JSON twin.
huma_dump_json() {
    local entry="$1"
    local label="${entry%%|*}"
    local file="${ROOT_DIR}/${entry#*|}"

    if [ ! -f "${file}" ]; then
        fail "Missing Huma dump for '${label}' at ${file}. Run 'make generate-docs' first."
    fi

    NODE_PATH="${OPENAPI_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        process.stdout.write(JSON.stringify(yaml.load(fs.readFileSync(process.argv[1], "utf8"))));
    ' "${file}"
}

# Read a Huma dump field as canonical JSON (sorted keys) for byte comparison.
read_field() {
    local entry="$1"
    local jq_filter="$2"

    huma_dump_json "${entry}" | jq -cS "${jq_filter}"
}

# Read a Huma dump scalar field as a raw (unquoted) string for regex matching.
read_field_raw() {
    local entry="$1"
    local jq_filter="$2"

    huma_dump_json "${entry}" | jq -r "${jq_filter}"
}

# Assert a field is byte-identical across all parity dumps.
assert_field_parity() {
    local field_label="$1"
    local jq_filter="$2"

    local reference_entry="${PARITY_DUMPS[0]}"
    local reference_label="${reference_entry%%|*}"
    local reference_value
    reference_value="$(read_field "${reference_entry}" "${jq_filter}")"

    local entry label value
    for entry in "${PARITY_DUMPS[@]:1}"; do
        label="${entry%%|*}"
        value="$(read_field "${entry}" "${jq_filter}")"
        if [ "${value}" != "${reference_value}" ]; then
            echo -e "    ${RED}❌ Field '${field_label}' diverged between '${reference_label}' and '${label}':${NC}"
            echo -e "       ${reference_label}: ${reference_value}"
            echo -e "       ${label}: ${value}"
            exit 1
        fi
    done

    ok "Field '${field_label}' is identical across: $(dump_labels "${PARITY_DUMPS[@]}")"
}

# Assert a field matches a regex in every parity dump.
assert_field_matches() {
    local field_label="$1"
    local jq_filter="$2"
    local regex="$3"

    local entry label value
    for entry in "${PARITY_DUMPS[@]}"; do
        label="${entry%%|*}"
        value="$(read_field_raw "${entry}" "${jq_filter}")"
        if ! [[ "${value}" =~ ${regex} ]]; then
            fail "Field '${field_label}' in dump '${label}' is '${value}', expected to match /${regex}/."
        fi
    done

    ok "Field '${field_label}' matches /${regex}/ across: $(dump_labels "${PARITY_DUMPS[@]}")"
}

parity_check() {
    print_header "Parity check (Huma dump shared metadata)"

    # .info.version is the one metadata field every dump emits AND must agree on
    # (a joined spec with mismatched versions is nonsense). Byte-identical parity
    # plus the ^4.0.0$ shape covers both "they agree" and "they are the release".
    assert_field_parity "info.version" '.info.version'
    assert_field_matches "info.version" '.info.version' '^4\.0\.0$'

    # Title is NOT byte-parity metadata — each dump names itself ("Midaz Ledger
    # API", "Midaz Tracer API") — but all MUST carry the runtime "Midaz" brand,
    # never a golden-test placeholder. ^Midaz catches a fixture title (e.g.
    # "contract-spec") leaking into a published dump.
    assert_field_matches "info.title" '.info.title' '^Midaz'

    # contact/license/termsOfService/schemes (swaggo-era) are honestly dropped —
    # Huma emits only .info.{title,version}, and OAS 3.1 has no .schemes.
}

# Dumps whose every operation must declare a .security requirement, in the same
# "<label>|<repo-relative path>" form as PARITY_DUMPS. The ledger dump carries the
# full ledger surface — both the /v1 and /v2 paths served by the same binary behind
# the same auth chain — so a single entry covers every ledger operation.
# assert_security_coverage_complete enforces that this list stays exhaustive.
SECURITY_COVERAGE_DUMPS=(
    "ledger|components/ledger/api/openapi.huma.yaml"
)

# Glob of every published ledger contract. A dump matching this that is not on
# SECURITY_COVERAGE_DUMPS would be converted into the collection and shipped
# without ever being security-checked.
LEDGER_DUMP_GLOB="components/ledger/api/openapi*.huma.yaml"

# Assert SECURITY_COVERAGE_DUMPS names every published ledger contract. The
# reverse direction (a listed dump that does not exist) already hard-fails in
# huma_dump_json.
assert_security_coverage_complete() {
    local dump rel entry covered

    for dump in "${ROOT_DIR}"/${LEDGER_DUMP_GLOB}; do
        [ -f "${dump}" ] || continue

        rel="${dump#"${ROOT_DIR}/"}"
        covered=0
        for entry in "${SECURITY_COVERAGE_DUMPS[@]}"; do
            if [ "${entry#*|}" = "${rel}" ]; then
                covered=1
                break
            fi
        done

        if [ "${covered}" -eq 0 ]; then
            fail "Published ledger contract '${rel}' is not covered by the security check. Add it to SECURITY_COVERAGE_DUMPS."
        fi
    done
}

# Assert every operation in the ledger dumps carries a non-empty .security block.
security_coverage_check() {
    print_header "Security coverage check (every operation secured: $(dump_labels "${SECURITY_COVERAGE_DUMPS[@]}"))"

    assert_security_coverage_complete

    # Operations are the HTTP-verb keys under each path; an operation is unsecured
    # when its .security array is absent or empty.
    local op_filter='.paths | to_entries[] | .key as $path | .value | to_entries[]
        | select(.key | test("^(get|post|put|patch|delete|head|options)$"))
        | { path: $path, method: .key, security: (.value.security // []) }'

    local entry label json total secured grand_total=0
    for entry in "${SECURITY_COVERAGE_DUMPS[@]}"; do
        label="${entry%%|*}"

        # jq cannot read YAML; work off the JSON projection of the Huma dump.
        json="$(huma_dump_json "${entry}")"

        total="$(jq "[ ${op_filter} ] | length" <<<"${json}")"
        secured="$(jq "[ ${op_filter} | select(.security | length > 0) ] | length" <<<"${json}")"

        if [ "${secured}" != "${total}" ]; then
            echo -e "    ${RED}❌ ${label} has unsecured operations (${secured}/${total} secured):${NC}" >&2
            jq -r "${op_filter} | select(.security | length == 0) | \"       \(.method | ascii_upcase) \(.path)\"" <<<"${json}" >&2
            echo -e "    ${RED}Every ledger operation must declare a .security requirement.${NC}" >&2
            exit 1
        fi

        # A covered dump with no operations means the check silently verified
        # nothing — the aggregate count alone cannot distinguish that from a pass.
        if [ "${total}" -eq 0 ]; then
            fail "${label} yielded 0 operations. A covered dump with no operations is a defect, not a pass."
        fi

        ok "${label}: ${total} operations, all with a .security requirement."

        grand_total=$((grand_total + total))
    done

    ok "All ${grand_total} ledger operations declare a .security requirement."
}

# Lint the consolidated spec with @redocly/cli. The ruleset is `recommended`
# scoped by scripts/openapi/redocly.yaml, which relaxes ONLY rules whose
# findings are inherited from the per-component source specs or are structural
# artifacts of the join (each documented with a WHY in that file). A genuinely
# new merge-introduced problem (broken $ref, dropped security scheme, invalid
# structure) still fails this gate.
consolidated_lint_check() {
    print_header "Consolidated spec lint (redocly)"

    local redocly_bin="${OPENAPI_DIR}/node_modules/.bin/redocly"
    local consolidated_yaml="${ROOT_DIR}/api/midaz.openapi.yaml"
    local redocly_config="${OPENAPI_DIR}/redocly.yaml"

    # Gate on artifact + binary presence (mirrors how drift_check is opt-in):
    # when run standalone without a prior `make generate-docs`, the merged spec
    # or node_modules may be absent; skip-with-warning rather than hard-fail.
    if [ ! -f "${consolidated_yaml}" ]; then
        echo -e "    ${BLUE}ℹ️  Consolidated spec not found at ${consolidated_yaml}; skipping (run 'make generate-docs' first).${NC}"
        return 0
    fi

    if [ ! -x "${redocly_bin}" ]; then
        echo -e "    ${BLUE}ℹ️  @redocly/cli not installed at ${redocly_bin}; skipping (run 'make generate-docs' first).${NC}"
        return 0
    fi

    if (cd "${ROOT_DIR}" && "${redocly_bin}" lint api/midaz.openapi.yaml --config "${redocly_config}"); then
        ok "Consolidated spec passed redocly lint (recommended, scoped by redocly.yaml)."
    else
        fail "redocly lint failed on the consolidated spec; see findings above."
    fi
}

# Assert the JOINED spec carries exactly one canonical `Error` schema.
#
# The PRIMARY lock on the Error closure is the Go test tests/openapi
# (error_schema_parity_test.go): it proves the `Error` closure is byte-identical
# across the two per-plane Huma dumps. But that test reads the per-plane dumps,
# NOT the joined artifact. The joined spec (api/midaz.openapi.json,
# consumed by the Plan B SDK) is the output of `redocly join`. If the join ever
# collides two non-identical `Error` schemas, redocly de-dups by suffixing the
# second (`Error`, `Error2` / `Error-2` / `Error_2`). The Go test would not see
# that. This guard protects the published artifact against exactly that.
error_schema_singleton_check() {
    print_header "Error schema singleton check (joined spec)"

    local joined_json="${ROOT_DIR}/api/midaz.openapi.json"

    # Mirror consolidated_lint_check: standalone runs without a prior
    # `make generate-docs` may lack the joined artifact; skip-with-warning.
    if [ ! -f "${joined_json}" ]; then
        echo -e "    ${BLUE}ℹ️  Joined spec not found at ${joined_json}; skipping (run 'make generate-docs' first).${NC}"
        return 0
    fi

    # (1) canonical Error must exist.
    if [ "$(jq '.components.schemas | has("Error")' "${joined_json}")" != "true" ]; then
        fail "Joined spec is missing components.schemas.Error."
    fi

    # (2) no redocly dedup-suffixed siblings (Error2, Error-2, Error_2, ...).
    #     `Error` itself and unrelated names like `ErrorDetail` do not match.
    local dupes
    dupes="$(jq -r '.components.schemas | keys[] | select(test("^Error[-_]?[0-9]+$"))' "${joined_json}")"
    if [ -n "${dupes}" ]; then
        fail "Joined spec has dedup-suffixed Error schema(s) — redocly join collided a non-identical Error: ${dupes//$'\n'/, }"
    fi

    # (3) shape sanity: the RFC 9457 problem fields the SDK relies on.
    local missing
    missing="$(jq -r '
        ["type","title","status","detail","instance","code"]
        - (.components.schemas.Error.properties | keys)
        | join(", ")
    ' "${joined_json}")"
    if [ -n "${missing}" ]; then
        fail "Joined spec components.schemas.Error is missing expected RFC 9457 field(s): ${missing}."
    fi

    ok "Joined spec has exactly one canonical RFC 9457 Error schema (no join-induced duplication)."
}

# Every committed artifact generate-docs.sh writes, so none of them can fall behind
# the contracts they are generated from.
DRIFT_PATHSPEC=(
    # Trailing /** is required: a git pathspec containing a wildcard is matched
    # against the whole path, so 'components/*/api' matches no FILE at all.
    'components/*/api/**'
    api/midaz.openapi.yaml
    api/midaz.openapi.json
)

drift_check() {
    print_header "Drift check (regenerate and diff)"

    echo -e "    ${BLUE}⏳ Regenerating docs via generate-docs.sh...${NC}"
    if ! "${OPENAPI_DIR}/generate-docs.sh"; then
        fail "generate-docs.sh failed; cannot verify regeneration reproduces committed artifacts."
    fi

    if ! git -C "${ROOT_DIR}" diff --exit-code -- "${DRIFT_PATHSPEC[@]}"; then
        echo ""
        echo -e "    ${RED}❌ Regeneration changed committed docs artifacts. Changed paths:${NC}"
        git -C "${ROOT_DIR}" diff --name-only -- "${DRIFT_PATHSPEC[@]}" | sed 's/^/       /'
        echo -e "    ${RED}Run 'make generate-docs' and commit the result.${NC}"
        exit 1
    fi

    ok "Regeneration reproduces committed docs artifacts (no drift)."
}

main() {
    require_jq
    parity_check
    security_coverage_check
    error_schema_singleton_check
    consolidated_lint_check

    if [ "${CHECK_DOCS_REGEN:-}" = "1" ]; then
        drift_check
    else
        echo ""
        echo -e "    ${BLUE}ℹ️  Drift check skipped (set CHECK_DOCS_REGEN=1 to enable).${NC}"
    fi

    echo ""
    echo -e "${GREEN}🎉 Documentation guardrail checks passed!${NC}"
    echo ""
}

main "$@"
