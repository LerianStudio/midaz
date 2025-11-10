# Stage 1: Build the Go application
FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS builder

# Set working directory inside the container
WORKDIR /onboarding-app

# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the project files
COPY . .

# Build the Go binary for the target platform
# CGO is disabled for static builds
# GOARCH is extracted from TARGETPLATFORM to support multi-arch builds
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/onboarding/cmd/app/main.go

# Stage 2: Create a minimal runtime image using distroless
FROM gcr.io/distroless/static-debian12

# Copy the compiled binary from the builder stage
COPY --from=builder /app /app

# Copy the migration files required at runtime
COPY --from=builder /onboarding-app/components/onboarding/migrations /components/onboarding/migrations

EXPOSE 3000

# Set entrypoint to the compiled binary
ENTRYPOINT ["/app"]
