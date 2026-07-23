# =============================================================================
# Dedicated Postgres migration runner image for the Midaz tracer.
#
# Bundles the golang-migrate binary with the tracer SQL migrations and applies
# them once against the configured tracer database, then exits. Used by the
# deploy Job to apply the schema before the app boots.
#
# The build CONTEXT is the repository ROOT (not this directory), so every
# COPY path below is repo-root-relative. Build with:
#   docker build -f components/tracer/migrations-image/Dockerfile -t <tag> .
#
# readOnlyRootFilesystem / non-root safety:
#   - Runs as UID/GID 65532 (nonroot) so the Job can enforce runAsNonRoot.
#   - readOnlyRootFilesystem is safe: /migrations and the entrypoint script
#     are baked in via COPY, and migrate writes no state to the container
#     filesystem — migration progress lives in the Postgres schema_migrations
#     table. Root-owned, world-readable files are intentional: they are read
#     only, so any non-owner UID can execute the entrypoint and read the SQL.
# =============================================================================

FROM migrate/migrate:v4.19.1

LABEL org.opencontainers.image.title="Midaz Tracer Migrations" \
    org.opencontainers.image.description="Dedicated PostgreSQL migration runner for the Midaz tracer database" \
    org.opencontainers.image.vendor="Lerian Studio" \
    org.opencontainers.image.source="https://github.com/LerianStudio/midaz" \
    org.opencontainers.image.licenses="Elastic-2.0"

COPY components/tracer/migrations /migrations
COPY components/tracer/migrations-image/entrypoint.sh /migrate-entrypoint.sh

USER 65532:65532

ENTRYPOINT ["/migrate-entrypoint.sh"]
