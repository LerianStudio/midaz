FROM golang:1.23-alpine AS builder

WORKDIR /ledger-app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /ledger-app/components/ledger/migrations /components/ledger/migrations

EXPOSE 3000

ENTRYPOINT ["/app"]