FROM golang:1.23-alpine AS builder

WORKDIR /audit-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/audit/cmd/app/main.go

FROM alpine:latest

COPY --from=builder /app /app

EXPOSE 3005

ENTRYPOINT ["/app"]