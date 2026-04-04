#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "🔨 Building..."
go build -o bin/epbf-monitor ./cmd/epbf-monitor

echo "🚀 Starting with sudo (eBPF requires root)..."
sudo -E ./bin/epbf-monitor
