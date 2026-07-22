#!/bin/sh
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Migration runner entrypoint for the Midaz ledger.
#
# Applies BOTH the onboarding and the transaction migration sets, in that
# order, then exits. Each database is independent: it accepts either a
# prebuilt <DB>_DATABASE_URL override or the individual DB_<DB>_* env vars.
#
# When assembling from DB_<DB>_* vars, the password is percent-encoded so
# special characters (%, @, :, /, ?, #, &, +, space, [, ]) cannot break the
# DSN. `%` is encoded first so already-inserted escapes are not double-encoded.
#
# set -e aborts on the first `migrate` failure; migrate is idempotent, so a
# re-run resumes from where it stopped (progress lives in the Postgres
# schema_migrations table, not on disk).
#
# This runner is tenant-agnostic: per-tenant looping is a deploy-Job concern.
set -eu

# encode_password percent-encodes URI-reserved characters in stdin.
# `%` MUST be substituted first to avoid double-encoding later escapes.
encode_password() {
    sed \
        -e 's/%/%25/g' \
        -e 's/@/%40/g' \
        -e 's/:/%3A/g' \
        -e 's|/|%2F|g' \
        -e 's/?/%3F/g' \
        -e 's/#/%23/g' \
        -e 's/&/%26/g' \
        -e 's/+/%2B/g' \
        -e 's/ /%20/g' \
        -e 's/\[/%5B/g' \
        -e 's/\]/%5D/g'
}

# Onboarding database.
echo "applying onboarding migrations"
if [ -n "${ONBOARDING_DATABASE_URL:-}" ]; then
    ONBOARDING_DB="$ONBOARDING_DATABASE_URL"
else
    ONBOARDING_PWD_ENC=$(printf '%s' "${DB_ONBOARDING_PASSWORD:-}" | encode_password)
    ONBOARDING_DB="postgres://${DB_ONBOARDING_USER}:${ONBOARDING_PWD_ENC}@${DB_ONBOARDING_HOST}:${DB_ONBOARDING_PORT:-5432}/${DB_ONBOARDING_NAME}?sslmode=${DB_ONBOARDING_SSLMODE:-disable}"
fi
migrate -path /migrations/onboarding -database "$ONBOARDING_DB" up

# Transaction database.
echo "applying transaction migrations"
if [ -n "${TRANSACTION_DATABASE_URL:-}" ]; then
    TRANSACTION_DB="$TRANSACTION_DATABASE_URL"
else
    TRANSACTION_PWD_ENC=$(printf '%s' "${DB_TRANSACTION_PASSWORD:-}" | encode_password)
    TRANSACTION_DB="postgres://${DB_TRANSACTION_USER}:${TRANSACTION_PWD_ENC}@${DB_TRANSACTION_HOST}:${DB_TRANSACTION_PORT:-5432}/${DB_TRANSACTION_NAME}?sslmode=${DB_TRANSACTION_SSLMODE:-disable}"
fi
migrate -path /migrations/transaction -database "$TRANSACTION_DB" up

echo "migrations complete"
