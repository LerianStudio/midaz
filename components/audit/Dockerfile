FROM golang:1.23-alpine AS builder

WORKDIR /audit-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/audit/cmd/app/main.go

FROM alpine:latest

ARG USER=midaz
RUN apk add --update sudo

RUN adduser -D $USER \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER

USER $USER

COPY --from=builder /app /app

EXPOSE 3005

ENTRYPOINT ["/app"]