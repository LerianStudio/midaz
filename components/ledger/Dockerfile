FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS builder

WORKDIR /ledger-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the Go binary for the target platform
# CGO is disabled for static builds
# GOARCH is extracted from TARGETPLATFORM to support multi-arch builds
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/ledger/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

COPY --from=builder /ledger-app/components/onboarding/migrations /components/onboarding/migrations
COPY --from=builder /ledger-app/components/transaction/migrations /components/transaction/migrations

# Expose ports for both onboarding (3000) and transaction (3001) HTTP servers
EXPOSE 3000
EXPOSE 3001

ENTRYPOINT ["/app"]
