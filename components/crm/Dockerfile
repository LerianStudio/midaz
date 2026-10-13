# syntax=docker/dockerfile:1.4

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /crm-app

COPY go.mod go.sum ./
ENV GOPRIVATE=github.com/LerianStudio/*
RUN apk add --no-cache git
RUN --mount=type=secret,id=github_token \
    if [ -f /run/secrets/github_token ]; then \
      export GIT_CONFIG_COUNT=1 \
        GIT_CONFIG_KEY_0="url.https://x-access-token:$(cat /run/secrets/github_token)@github.com/.insteadOf" \
        GIT_CONFIG_VALUE_0="https://github.com/"; \
    fi && \
    go mod download

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

USER nonroot:nonroot

ENTRYPOINT ["/app"]
