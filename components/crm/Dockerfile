# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS builder

WORKDIR /crm-app

# Copy local dependencies from parent context
COPY lib-commons /lib-commons
COPY lib-auth /lib-auth

# Copy Go module files
COPY midaz/go.mod midaz/go.sum ./

# Create symlinks so ../lib-commons and ../lib-auth work
RUN ln -s /lib-commons ../lib-commons && ln -s /lib-auth ../lib-auth

RUN go mod download

# Copy application source code
COPY midaz/ .

# Remove go.work if present (local dev only)
RUN rm -f go.work go.work.sum

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app components/crm/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

EXPOSE 4003

ENTRYPOINT ["/app"]
