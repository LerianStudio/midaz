FROM golang:1.23-alpine AS builder

WORKDIR /ledger-app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/ledger/cmd/app/main.go

FROM alpine:latest

ARG USER=midaz
RUN apk add --update sudo

RUN adduser -D $USER \
        && echo "$USER ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/$USER \
        && chmod 0440 /etc/sudoers.d/$USER

USER $USER

COPY --from=builder /app /app

# Copy the migrations directory.
COPY --from=builder /ledger-app/components/ledger/migrations /components/ledger/migrations

EXPOSE 3000 3001

ENTRYPOINT ["/app"]