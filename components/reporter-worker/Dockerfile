FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine AS builder

WORKDIR /reporter-worker-app

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
    -o /worker components/reporter-worker/cmd/app/main.go

# Worker renders PDFs via chromedp, so it MUST stay fat alpine + headless
# Chromium (distroless cannot host the Chromium userland — R20).
FROM alpine:3.23

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Reporter Worker - Async Report Generator"
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

# Copy the binary from the builder stage
COPY --from=builder /worker /app/worker

# Switch to non-root user
USER appuser

# Expose health check port for container orchestrators
EXPOSE 4006

# Health check via the worker's HTTP health endpoint (alpine keeps a shell).
HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:4006/health || exit 1

# Set the entrypoint to the worker binary
ENTRYPOINT ["/app/worker"]
