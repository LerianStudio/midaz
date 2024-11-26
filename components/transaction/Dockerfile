FROM golang:1.23-alpine AS builder

WORKDIR /transaction-app

COPY . .

RUN CGO_ENABLE=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/transaction/cmd/app/main.go

# Add make, install swag and generate docs
RUN apk add --no-cache make  \
    && go install github.com/swaggo/swag/cmd/swag@latest \
    && make -C components/transaction generate-docs


FROM alpine:latest

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3002

ENTRYPOINT ["/app"]