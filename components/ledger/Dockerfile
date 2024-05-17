FROM golang:1.21-alpine AS builder

WORKDIR /ledger-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/internal/main.go

FROM gcr.io/distroless/static-debian12

COPY --chown=nonroot:nonroot --from=builder /app /app

# Copy the migrations directory.
COPY --chown=nonroot:nonroot --from=builder /ledger-app/components/ledger/migrations /components/ledger/migrations

USER nonroot

EXPOSE 3000

ENTRYPOINT ["/app"]