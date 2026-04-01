#!/bin/bash
# Build script for network-monitor plugin

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$PLUGIN_DIR/build"
OUTPUT_DIR="$PLUGIN_DIR/output"

echo "🔨 Building network-monitor plugin..."
echo "   Source: $PLUGIN_DIR"
echo "   Build:  $BUILD_DIR"
echo "   Output: $OUTPUT_DIR"

# Create directories
mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

# Check for clang
if ! command -v clang &> /dev/null; then
    echo "❌ clang not found. Please install clang."
    exit 1
fi

echo ""
echo "📦 Building eBPF program..."
clang -O2 -g -Wall -Wextra \
    -target bpf \
    -D__TARGET_ARCH_x86_64 \
    -I/usr/include \
    -c "$PLUGIN_DIR/ebpf/main.c" \
    -o "$BUILD_DIR/program.o"
echo "✅ eBPF: $BUILD_DIR/program.o"

echo ""
echo "📦 Building WASM module..."
clang -O2 -g -Wall -Wextra \
    --target=wasm32 \
    -nostdlib \
    -Wl,--no-entry \
    -Wl,--export=epbf_init \
    -Wl,--export=process_events \
    -Wl,--export=epbf_cleanup \
    -Wl,--export=__data_end \
    -Wl,--export=__heap_base \
    -Wl,--strip-debug \
    -Wl,--allow-undefined \
    -I"$PLUGIN_DIR/../../pkg/wasmsdk/include" \
    "$PLUGIN_DIR/wasm/main.c" \
    -o "$BUILD_DIR/plugin.wasm"
echo "✅ WASM: $BUILD_DIR/plugin.wasm"

echo ""
echo "📊 Build artifacts:"
ls -lh "$BUILD_DIR/"

echo ""
echo "✅ Build complete!"
echo ""
echo "To test locally:"
echo "  1. Copy build/program.o and build/plugin.wasm to your epbf-monitoring instance"
echo "  2. Or use: curl -X POST http://localhost:8080/api/v1/plugins \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"git_url\": \"file://$PLUGIN_DIR\"}'"
