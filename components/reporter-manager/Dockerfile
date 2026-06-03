# syntax=docker/dockerfile:1.4 

FROM golang:1.26-alpine AS builder

# Set the working directory for the build
WORKDIR /reporter

# Copy only go.mod and go.sum first to cache dependencies
COPY go.mod go.sum ./

# Download Go modules
RUN go mod download

# Copy only necessary source files for manager component
COPY pkg/ pkg/
COPY components/manager/ components/manager/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -tags netgo \
  -ldflags='-w -s -extldflags "-static"' \
  -o /manager ./components/manager/cmd/app/main.go

# Stage 2: Final image (Manager is REST API only, no PDF generation needed)
FROM alpine:3.23

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/reporter"
LABEL org.opencontainers.image.description="Reporter Manager API"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

WORKDIR /app

# Install only essential runtime dependencies
RUN apk add --no-cache ca-certificates && rm -rf /var/cache/apk/*

# Create a non-root user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Copy the binary from the builder stage
COPY --from=builder /manager /app/manager

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose the application port
EXPOSE 4005

# Health check for container orchestrators
HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
  CMD wget --no-verbose --tries=1 --spider http://localhost:4005/health || exit 1

# Set the entrypoint to the manager binary
ENTRYPOINT ["/app/manager"]

