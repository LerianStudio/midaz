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

# The migrator resolves its source via filepath.Abs(os.Getwd()) applied to the
# CWD-relative "components/ledger/migrations/...". The :nonroot base runs with
# CWD /home/nonroot, so stage the migrations there and pin WORKDIR to match.
WORKDIR /home/nonroot

COPY --from=builder /app /app
COPY --chown=nonroot:nonroot --from=builder \
  /ledger-app/components/ledger/migrations/onboarding \
  /home/nonroot/components/ledger/migrations/onboarding
COPY --chown=nonroot:nonroot --from=builder \
  /ledger-app/components/ledger/migrations/transaction \
  /home/nonroot/components/ledger/migrations/transaction

EXPOSE 3002

USER nonroot:nonroot

ENTRYPOINT ["/app"]

