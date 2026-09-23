FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder

WORKDIR /tracer-app

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && update-ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
ARG BUILD_TIME=unknown
SHELL ["/bin/ash", "-e", "-o", "pipefail", "-c"]
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -buildvcs=false -tags netgo \
    -ldflags "-s -w -extldflags '-static' -X main.version=${VERSION} -X main.revision=${REVISION} -X main.buildTime=${BUILD_TIME}" \
    -o /app/tracer ./components/tracer/cmd/app

# Production image using distroless for minimal attack surface (Ring Standards)
# Note: distroless has no shell - use orchestrator health checks (Kubernetes probes)
FROM gcr.io/distroless/static-debian12:nonroot AS prod

ARG VERSION=dev
ARG BUILD_TIME=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Midaz Tracer (real-time transaction validation + fraud prevention)"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"

WORKDIR /app

COPY --from=builder /app/tracer /app/tracer

# Explicitly set non-root user for Docker Hub health score compliance
USER nonroot:nonroot

# GOMEMLIMIT — soft memory limit for the Go runtime. Set to ~90% of the
# container memory limit so the GC triggers before the kernel OOM-kills the
# pod. Default 1800MiB assumes a 2 GiB container limit; override in Helm /
# Kustomize to match your deployment's `resources.limits.memory`:
#   GOMEMLIMIT = resources.limits.memory × 0.9
ENV GOMEMLIMIT=1800MiB

EXPOSE 4020

ENTRYPOINT ["/app/tracer"]
