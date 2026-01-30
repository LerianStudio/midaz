# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS builder

WORKDIR /crm-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app components/crm/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

EXPOSE 4003

ENTRYPOINT ["/app"]