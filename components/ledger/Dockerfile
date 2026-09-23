FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder

WORKDIR /ledger-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -buildvcs=false -tags netgo \
    -ldflags "-s -w -extldflags '-static' -X main.version=${VERSION} -X main.revision=${REVISION} -X main.buildTime=${BUILD_TIME}" \
    -o /app ./components/ledger/cmd/app

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG BUILD_TIME=unknown

LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"
LABEL org.opencontainers.image.description="Midaz Ledger (unified binary: onboarding + transaction + CRM + fees)"
LABEL org.opencontainers.image.vendor="Lerian Studio"
LABEL org.opencontainers.image.licenses="Elastic-2.0"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"

COPY --from=builder /app /app

EXPOSE 3002

USER nonroot:nonroot

ENTRYPOINT ["/app"]

