FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

WORKDIR /transaction-app

# Copy go.mod and go.sum from core directory
COPY core/go.mod core/go.sum ./

# Copy lib dependencies to satisfy replace directives
COPY libs/ ./libs/

# Fix replace paths for Docker build context
RUN sed -i 's|../libs/|./libs/|g' go.mod

RUN go mod download

# Copy the entire core directory
COPY core/ ./

# Fix replace paths again in case they were overwritten
RUN sed -i 's|../libs/|./libs/|g' go.mod

ARG TARGETPLATFORM
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2) go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /app components/transaction/cmd/app/main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app
COPY --from=builder /transaction-app/components/transaction/migrations /components/transaction/migrations

EXPOSE 3001
ENTRYPOINT ["/app"]