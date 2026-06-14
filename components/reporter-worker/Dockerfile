# Image-name anchor. The reporter binary was unified into components/reporter
# (RUN_MODE=api|worker|all); this directory survives ONLY to host this Dockerfile
# because the shared CI build workflow derives the published image name
# (midaz-reporter-worker) from the build-context directory basename. Keeping the
# name stable lets the existing Helm chart (.worker image key) stay untouched —
# devops owns any later rename. There is no Go source here; it lives one level up.

FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder

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

# ONE unified binary, all RUN_MODEs. This image is the worker surface: it builds
# the SAME components/reporter binary as the API image and differs only by base
# image (fat alpine + Chromium) and the default RUN_MODE=worker. Production
# splits api/worker into two Deployments from these two images of one component.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /worker components/reporter/cmd/app/main.go

# Worker surface renders PDFs via chromedp, so it MUST stay fat alpine + headless
# Chromium (distroless cannot host the Chromium userland — R20).
FROM alpine:3.23

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Reporter Worker - Async Report Generator (unified reporter binary, RUN_MODE=worker)"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

WORKDIR /app

# Install Chromium and all required dependencies for chromedp
RUN apk add --no-cache \
    ca-certificates \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    ttf-dejavu \
    ttf-liberation \
    font-noto \
    font-noto-cjk \
    font-noto-emoji \
    font-noto-extra \
    && rm -rf /var/cache/apk/*

# Create a non-root user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S -h /app appuser -G appgroup && \
    mkdir -p /app/.chromium && \
    chown -R appuser:appgroup /app

# Set Chrome/Chromium environment variables for headless operation
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/bin/chromium-browser
ENV CHROMEDP_SKIP_CHROMEDP_DOWNLOAD=true
ENV CHROMEDP_USE_SYSTEM_CHROME=true
ENV HOME=/app

# Default this image to the worker surface. Production sets RUN_MODE=worker
# explicitly per Deployment; baking it here keeps `docker run` of this image
# worker-only.
ENV RUN_MODE=worker

# Copy the binary from the builder stage
COPY --from=builder /worker /app/worker

# Switch to non-root user
USER appuser

# Expose health check port for container orchestrators
EXPOSE 4006

# Health check via the worker's HTTP health endpoint (alpine keeps a shell).
HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:4006/health || exit 1

# Set the entrypoint to the unified reporter binary (RUN_MODE=worker by default).
ENTRYPOINT ["/app/worker"]
