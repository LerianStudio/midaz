FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS builder

WORKDIR /transaction-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/transaction/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

COPY --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3001 3011

# Run the application
ENTRYPOINT ["/app"]