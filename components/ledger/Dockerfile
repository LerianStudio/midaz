FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS builder

WORKDIR /ledger-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/ledger/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app
COPY --from=builder /ledger-app/components/ledger/migrations/onboarding /components/ledger/migrations/onboarding
COPY --from=builder /ledger-app/components/ledger/migrations/transaction /components/ledger/migrations/transaction

EXPOSE 3002

USER nonroot:nonroot

ENTRYPOINT ["/app"]


