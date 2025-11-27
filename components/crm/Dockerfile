# syntax=docker/dockerfile:1.4

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS builder

# Install git and garble for private module access and obfuscation
RUN apk add --no-cache git && \
    go install mvdan.cc/garble@latest

# Set working directory
WORKDIR /crm-app

# Copy Go module files to take advantage of layer caching
COPY go.mod go.sum ./
RUN go mod download

# Use BuildKit secret to authenticate with GitHub and download private dependencies
RUN --mount=type=secret,id=github_token \
  GITHUB_TOKEN=$(cat /run/secrets/github_token) && \
  printf "machine github.com\nlogin x-oauth-basic\npassword %s\n" "$GITHUB_TOKEN" > ~/.netrc && \
  go mod download && \
  rm ~/.netrc

# Copy the full source code
COPY . .

# Add Go bin path for garble and build the binary with obfuscation (multi-arch)
ENV PATH="/root/go/bin:${PATH}"
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    garble -tiny -literals build -a -tags netgo \
    -ldflags='-w -extldflags "-static"' \
    -o /app components/crm/cmd/app/main.go

# Final minimal image (Distroless)
FROM gcr.io/distroless/static-debian12

# Copy the obfuscated binary into the final image
COPY --from=builder /app /app

# Expose application port
EXPOSE 4003

# Define entrypoint
ENTRYPOINT ["/app"]