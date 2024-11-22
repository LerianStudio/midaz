FROM golang:1.23-alpine AS builder

WORKDIR /ledger-two-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/cmd/main.go

FROM alpine:latest

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /ledger-app/components/ledger/migrations /components/ledger/migrations

EXPOSE 3000 3001

ENTRYPOINT ["/app"]