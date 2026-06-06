FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine AS builder

WORKDIR /reporter-manager-app

# VCS build-info stamping needs the git binary; .git is in the root build context.
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

# Repo-root build context. Reporter imports ZERO midaz packages, so Go's
# package-level dead-code elimination keeps uncopied midaz packages out of the
# build graph. `COPY . .` is the only future-proof option: a selective COPY
# would break the moment any future reporter code imports a midaz pkg.
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/reporter-manager/cmd/app/main.go

# Manager is REST-only (no Chromium/PDF) — distroless mirrors ledger/tracer.
# distroless has no shell, so no embedded HEALTHCHECK; rely on the orchestrator
# probe against /health (mirrors ledger's distroless image, which has none).
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Reporter Manager API"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder /app /app

USER nonroot:nonroot

EXPOSE 4005

ENTRYPOINT ["/app"]
