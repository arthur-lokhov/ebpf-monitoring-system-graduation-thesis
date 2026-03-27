#!/bin/bash
# Build plugin in Docker container

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME=$(basename "$PLUGIN_DIR")
BUILD_DIR="$PLUGIN_DIR/build"

echo "🔨 Building $PLUGIN_NAME plugin in Docker..."

# Create build directory
mkdir -p "$BUILD_DIR"

# Run builder container
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"

docker run --rm --entrypoint /bin/sh \
  -v "$PLUGIN_DIR:/workspace/plugin" \
  -v "$BUILD_DIR:/workspace/output" \
  -w /workspace/plugin \
  epbf-monitor-builder:latest \
  -c "
    set -e
    echo '📦 Building eBPF program...'
    clang -O2 -g -Wall -Wextra \
      -target bpf \
      -D__TARGET_ARCH_x86_64 \
      -I/usr/include \
      -c ebpf/main.c \
      -o /workspace/output/program.o
    echo '✅ eBPF: /workspace/output/program.o'
    
    echo ''
    echo '📦 Building WASM module...'
    clang -O2 -g -Wall -Wextra \
      --target=wasm32 \
      -nostdlib \
      -Wl,--no-entry \
      -Wl,--export=epbf_init \
      -Wl,--export=__data_end \
      -Wl,--export=__heap_base \
      -Wl,--strip-debug \
      -Wl,--allow-undefined \
      -I/workspace/plugin/../../pkg/wasmsdk/include \
      wasm/main.c \
      -o /workspace/output/plugin.wasm
    echo '✅ WASM: /workspace/output/plugin.wasm'
    
    echo ''
    echo '📊 Build artifacts:'
    ls -lh /workspace/output/
  "

echo ""
echo "✅ Build complete!"
echo ""
echo "Artifacts:"
ls -lh "$BUILD_DIR/"
