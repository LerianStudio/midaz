#!/bin/sh
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Migration runner entrypoint for the Midaz tracer.
#
# Applies the tracer migration set against its single database, then exits.
# It accepts either a prebuilt DATABASE_URL override or the individual DB_*
# env vars.
#
# When assembling from DB_* vars, the password is percent-encoded so special
# characters (%, @, :, /, ?, #, &, +, space, [, ]) cannot break the DSN. `%`
# is encoded first so already-inserted escapes are not double-encoded.
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

# Tracer database.
echo "applying tracer migrations"
if [ -n "${DATABASE_URL:-}" ]; then
    TRACER_DB="$DATABASE_URL"
else
    TRACER_PWD_ENC=$(printf '%s' "${DB_PASSWORD:-}" | encode_password)
    TRACER_DB="postgres://${DB_USER}:${TRACER_PWD_ENC}@${DB_HOST}:${DB_PORT:-5432}/${DB_NAME}?sslmode=${DB_SSL_MODE:-disable}"
fi
migrate -path /migrations -database "$TRACER_DB" up

echo "migrations complete"
