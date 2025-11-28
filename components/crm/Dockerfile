# syntax=docker/dockerfile:1.4

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS builder

# Set working directory
WORKDIR /crm-app

# Copy Go module files to take advantage of layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source code
COPY . .

# Build the binary (multi-arch)
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    go build -tags netgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app components/crm/cmd/app/main.go

# Final minimal image (Distroless)
FROM gcr.io/distroless/static-debian12

# Copy the binary into the final image
COPY --from=builder /app /app

# Expose application port
EXPOSE 4003

# Define entrypoint
ENTRYPOINT ["/app"]