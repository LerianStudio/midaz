FROM golang:1.23-alpine AS builder

WORKDIR /audit-app

COPY . .

RUN CGO_ENABLE=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/audit/cmd/app/main.go

# Add make, install swag and generate docs
RUN apk add --no-cache make  \
    && go install github.com/swaggo/swag/cmd/swag@latest \
    && make -C components/audit generate-docs

FROM alpine:latest

COPY --from=builder /app /app

EXPOSE 3005

ENTRYPOINT ["/app"]