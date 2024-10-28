FROM golang:1.23-alpine AS builder

WORKDIR /transaction-app

COPY . .

RUN CGO_ENABLE=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/transaction/internal/main.go

FROM gcr.io/distroless/static-debian12

COPY --chown=nonroot:nonroot --from=builder /app /app

# Copy the migrations directory.
COPY --chown=nonroot:nonroot --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

USER nonroot

EXPOSE 3002

ENTRYPOINT ["/app"]