FROM golang:1.23-alpine AS builder

WORKDIR /ledger-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/cmd/app/main.go
# Add make, install swag and generate docs
RUN apk add --no-cache make  \
    && go install github.com/swaggo/swag/cmd/swag@latest \
    && make -C components/ledger generate-docs

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/internal/main.go

FROM alpine:latest

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /ledger-app/components/ledger/migrations /components/ledger/migrations

EXPOSE 3000 3001

ENTRYPOINT ["/app"]