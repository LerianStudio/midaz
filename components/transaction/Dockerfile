FROM golang:1.23-alpine AS builder

WORKDIR /transaction-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/transaction/cmd/app/main.go

FROM alpine:latest

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3002

ENTRYPOINT ["/app"]