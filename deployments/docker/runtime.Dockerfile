FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies (gcc required for wasmtime-go CGO)
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build with CGO enabled (required for wasmtime-go)
RUN CGO_ENABLED=1 GOOS=linux go build -o /epbf-monitor ./cmd/epbf-monitor

# Runtime image
FROM alpine:3.18

WORKDIR /app

# Install ca-certificates for HTTPS, docker-cli for building, and git for cloning plugins
RUN apk --no-cache add ca-certificates curl docker-cli git

# Copy binary
COPY --from=builder /epbf-monitor /app/epbf-monitor

# Copy migrations
COPY internal/storage/postgres/migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/app/epbf-monitor"]
