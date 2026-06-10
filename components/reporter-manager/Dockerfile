# Image-name anchor. The reporter binary was unified into components/reporter
# (RUN_MODE=api|worker|all); this directory survives ONLY to host this Dockerfile
# because the shared CI build workflow derives the published image name
# (midaz-reporter-manager) from the build-context directory basename. Keeping the
# name stable lets the existing Helm chart (.manager image key) stay untouched —
# devops owns any later rename. There is no Go source here; it lives one level up.

FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine AS builder

WORKDIR /reporter-app

# VCS build-info stamping needs the git binary; .git is in the root build context.
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

# Repo-root build context (CI build context defaults to '.'; the compose build
# context is ../../). Reporter imports ZERO midaz packages, so Go's
# package-level dead-code elimination keeps uncopied midaz packages out of the
# build graph. `COPY . .` is the only future-proof option: a selective COPY
# would break the moment any future reporter code imports a midaz pkg.
COPY . .

# ONE unified binary, all RUN_MODEs. This image is the API surface: it builds
# the SAME components/reporter binary as the worker image and differs only by
# base image (distroless, no Chromium) and the default RUN_MODE=api. Production
# splits api/worker into two Deployments from these two images of one component.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/reporter/cmd/app/main.go

# API surface is REST-only (no Chromium/PDF) — distroless mirrors ledger/tracer.
# distroless has no shell, so no embedded HEALTHCHECK; rely on the orchestrator
# probe against /health (mirrors ledger's distroless image, which has none).
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Reporter API (unified reporter binary, RUN_MODE=api)"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder /app /app

# Default this image to the API surface. Production sets RUN_MODE=api explicitly
# per Deployment; baking it here keeps `docker run` of this image API-only.
ENV RUN_MODE=api

USER nonroot:nonroot

EXPOSE 4005

ENTRYPOINT ["/app"]
