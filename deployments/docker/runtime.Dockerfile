FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Install build dependencies (gcc required for wasmtime-go CGO)
RUN apt-get update && apt-get install -y --no-install-recommends git gcc libc6-dev && rm -rf /var/lib/apt/lists/*

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
FROM golang:1.25-bookworm

WORKDIR /app

# Install ca-certificates for HTTPS, docker-cli for building, and git for cloning plugins
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl docker.io git && rm -rf /var/lib/apt/lists/*

# Copy binary
COPY --from=builder /epbf-monitor /app/epbf-monitor

# Copy migrations
COPY internal/storage/postgres/migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/app/epbf-monitor"]
