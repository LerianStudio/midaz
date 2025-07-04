FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /consumer-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/consumer/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app
COPY --from=builder /consumer-app/components/consumer/migrations /components/consumer/migrations

ENTRYPOINT ["/app"]