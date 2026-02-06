FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS builder

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
COPY --from=builder /ledger-app/components/onboarding/migrations /components/onboarding/migrations
COPY --from=builder /ledger-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3002

ENTRYPOINT ["/app"]
