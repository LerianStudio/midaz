FROM golang:1.23-alpine AS builder

WORKDIR /audit-app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/audit/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

EXPOSE 3002

ENTRYPOINT ["/app"]