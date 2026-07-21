FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine AS builder

WORKDIR /ledger-app

# VCS build-info stamping needs the git binary; .git is in the root build context.
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/ledger/cmd/app/main.go

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Midaz Ledger (unified binary: onboarding + transaction + CRM + fees)"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder /app /app
COPY --from=builder /ledger-app/components/ledger/migrations/onboarding /components/ledger/migrations/onboarding
COPY --from=builder /ledger-app/components/ledger/migrations/transaction /components/ledger/migrations/transaction

EXPOSE 3002

USER nonroot:nonroot

ENTRYPOINT ["/app"]

