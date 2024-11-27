FROM golang:1.23-alpine AS builder

WORKDIR /audit-app

COPY . .

RUN CGO_ENABLE=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/audit/cmd/app/main.go

FROM alpine:latest

COPY --from=builder /app /app

EXPOSE 3003

ENTRYPOINT ["/app"]