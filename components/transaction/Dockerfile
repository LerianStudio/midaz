FROM golang:1.23-alpine AS builder

WORKDIR /transaction-app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/transaction/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3001

ENTRYPOINT ["/app"]