FROM golang:1.26.3-alpine3.22 AS builder

WORKDIR /app

RUN apk add --no-cache \
    ca-certificates \
    git \
    tzdata \
    && update-ca-certificates

WORKDIR /tracer

# Copy only go.mod and go.sum first to cache dependencies
COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG TARGETARCH
SHELL ["/bin/ash", "-e", "-o", "pipefail", "-c"]
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app/tracer ./cmd/app
# Production image using distroless for minimal attack surface (Ring Standards)
# Note: distroless has no shell - use orchestrator health checks (Kubernetes probes)
FROM gcr.io/distroless/static-debian12:nonroot AS prod

WORKDIR /app

COPY --from=builder /app/tracer /app/tracer
COPY --from=builder /tracer/migrations /app/migrations

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



