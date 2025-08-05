FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /onboarding-app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) go build -tags netgo -ldflags '-s -w -extldflags "-static"' -o /app components/onboarding/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app
COPY --from=builder /onboarding-app/components/onboarding/migrations /components/onboarding/migrations

EXPOSE 3000
ENTRYPOINT ["/app"]