# syntax=docker/dockerfile:1.4 

FROM golang:1.26-alpine AS builder

# Set the working directory for the build
WORKDIR /reporter

# Copy only go.mod and go.sum first to cache dependencies
COPY go.mod go.sum ./

# Download Go modules
RUN go mod download

# Copy only necessary source files for worker component
COPY pkg/ pkg/
COPY components/worker/ components/worker/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -tags netgo \
  -ldflags='-w -s -extldflags "-static"' \
  -o /worker ./components/worker/cmd/app/main.go

# Stage 2: Final minimal image with only the binary
FROM alpine:3.23

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/reporter"
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

# Health check via the worker's HTTP health endpoint (checks RabbitMQ connectivity).
HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:4006/health || exit 1

# Set the entrypoint to the worker binary
ENTRYPOINT ["/app/worker"]