FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    clang \
    llvm \
    lld \
    musl-dev \
    linux-headers \
    git \
    make \
    bash \
    libbpf-dev

# Install eBPF target
RUN clang --version | head -1

# Install WASM target
RUN clang --target=wasm32 --version 2>&1 | head -1 || true

# Create builder user
RUN adduser -D -u 1000 builder
USER builder

WORKDIR /workspace

# Default command
ENTRYPOINT []
CMD ["/bin/sh"]
