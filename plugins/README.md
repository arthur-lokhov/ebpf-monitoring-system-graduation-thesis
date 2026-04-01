# eBPF Monitoring Plugins

Collection of example eBPF+WASM plugins for the epbf-monitoring platform.

## Available Plugins

### 1. Container Monitor (`example-container-monitor/`)
Monitors Docker container lifecycle events.

**Metrics:**
- `container_starts_total` - Total container starts
- `container_stops_total` - Total container stops
- `network_connections_total` - Network connections from containers
- `active_containers` - Current active containers

**eBPF Programs:**
- `trace_container_start` - Tracepoint on sys_enter_execve
- `trace_container_stop` - Tracepoint on sys_enter_kill
- `trace_network_connect` - Tracepoint on sys_enter_connect

### 2. Network Monitor (`plugin-network-monitor/`)
Monitors TCP connections and network traffic.

**Metrics:**
- `tcp_connections_total` - Total TCP connections
- `bytes_sent_total` - Total bytes sent
- `bytes_received_total` - Total bytes received
- `connection_duration_seconds` - Connection duration histogram
- `active_connections` - Current active connections

**eBPF Programs:**
- `trace_tcp_connect` - Tracepoint on sys_enter_connect
- `trace_tcp_sendmsg` - Kprobe on tcp_sendmsg
- `trace_tcp_recvmsg` - Kprobe on tcp_recvmsg

**Example Queries:**
```promql
# Connections per second
rate(tcp_connections_total[1m])

# Bytes per second by port
sum by (dest_port) (rate(bytes_sent_total[1m]))

# 99th percentile connection duration
histogram_quantile(0.99, connection_duration_seconds)
```

### 3. Disk Monitor (`plugin-disk-monitor/`)
Monitors disk I/O operations.

**Metrics:**
- `disk_read_bytes_total` - Total bytes read
- `disk_write_bytes_total` - Total bytes written
- `disk_read_ops_total` - Total read operations
- `disk_write_ops_total` - Total write operations
- `disk_io_size_bytes` - I/O size histogram
- `disk_queue_length` - Current queue length

**eBPF Programs:**
- `trace_vfs_read` - Kprobe on vfs_read
- `trace_vfs_write` - Kprobe on vfs_write
- `trace_block_rq_issue` - Tracepoint on block_rq_issue

**Example Queries:**
```promql
# Read throughput
rate(disk_read_bytes_total[1m])

# IOPS (I/O operations per second)
sum(rate(disk_read_ops_total[1m])) + sum(rate(disk_write_ops_total[1m]))

# 50th percentile I/O size
histogram_quantile(0.50, disk_io_size_bytes)
```

### 4. Process Monitor (`plugin-process-monitor/`)
Monitors process lifecycle and CPU usage.

**Metrics:**
- `process_starts_total` - Total process starts
- `process_exits_total` - Total process exits
- `process_duration_seconds` - Process duration histogram
- `active_processes` - Current active processes
- `cpu_time_seconds_total` - Total CPU time
- `context_switches_total` - Total context switches

**eBPF Programs:**
- `trace_sched_process_fork` - Tracepoint on sched_process_fork
- `trace_sched_process_exit` - Tracepoint on sched_process_exit
- `trace_sched_switch` - Tracepoint on sched_switch

**Example Queries:**
```promql
# Processes started per minute
per_minute(process_starts_total)

# Median process duration
histogram_quantile(0.50, process_duration_seconds)

# Context switches per second
rate(context_switches_total[1m])
```

## Plugin Structure

Each plugin follows this structure:

```
plugin-name/
├── manifest.yml          # Plugin metadata and metric definitions
├── ebpf/
│   └── main.c           # eBPF kernel-space code
├── wasm/
│   └── main.c           # WASM user-space code
├── build.sh             # Build script
└── README.md            # Plugin documentation
```

### manifest.yml

```yaml
name: plugin-name
version: 1.0.0
description: Plugin description
author: Author name

ebpf:
  entry: ebpf/main.c
  programs:
    - name: program_name
      type: tracepoint|kprobe|uprobe
      attach: tracepoint_name

wasm:
  entry: wasm/main.c
  sdk_version: "1.0"

metrics:
  - name: metric_name
    type: counter|gauge|histogram
    help: Metric description
    labels: [label1, label2]

filters:
  - name: filter_name
    expression: promql_expression
```

## Building Plugins

### Prerequisites

- Clang 14+ with BPF target support
- WASM32 target for Clang
- Linux headers

### Build Commands

```bash
# Build individual plugin
cd plugins/plugin-name
./build.sh

# Build all plugins
for dir in plugins/*/; do
  if [ -f "$dir/build.sh" ]; then
    (cd "$dir" && ./build.sh)
  fi
done
```

### Build Output

```
plugin-name/
├── build/
│   ├── program.o        # Compiled eBPF program
│   └── plugin.wasm      # Compiled WASM module
└── output/              # Final artifacts (after deployment)
```

## Installing Plugins

### Via UI

1. Open http://localhost:3000/plugins
2. Click "Add Plugin"
3. Enter Git repository URL or local path
4. Click "Add"

### Via API

```bash
# From Git repository
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "https://github.com/user/plugin-name"}'

# From local directory
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file:///path/to/plugin-name"}'
```

### Verify Installation

```bash
# Check plugin status
curl http://localhost:8080/api/v1/plugins

# View metrics
curl http://localhost:8080/api/v1/metrics

# Execute query
curl -X POST http://localhost:8080/api/v1/metrics/query \
  -H "Content-Type: application/json" \
  -d '{"query": "rate(metric_name[1m])"}'
```

## Developing Custom Plugins

### 1. Create Directory Structure

```bash
mkdir -p my-plugin/{ebpf,wasm}
```

### 2. Write manifest.yml

Define your metrics and eBPF programs.

### 3. Write eBPF Code

```c
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

SEC("tracepoint/...")
int trace_event(struct bpf_tracepoint *ctx) {
    // Your eBPF logic here
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
```

### 4. Write WASM Code

```c
#include <stdint.h>

__attribute__((export))
int epbf_init(void) {
    // Initialization code
    return 0;
}

__attribute__((export))
void process_events(void) {
    // Event processing loop
}

__attribute__((export))
void epbf_cleanup(void) {
    // Cleanup code
}
```

### 5. Build and Test

```bash
./build.sh
```

## Troubleshooting

### Build Errors

**"clang not found"**
```bash
# Install clang
sudo apt-get install clang llvm
```

**"bpf/bpf_helpers.h: No such file"**
```bash
# Install Linux headers
sudo apt-get install linux-headers-$(uname -r)
```

### Runtime Errors

**Plugin stuck in "pending" status**
- Check build logs in UI
- Verify eBPF program passes verifier
- Check for missing kernel functions

**No metrics appearing**
- Verify eBPF tracepoints exist on your kernel
- Check WASM logs for errors
- Ensure plugin is enabled

## Best Practices

1. **Keep eBPF programs simple** - Complex logic in WASM
2. **Use ring buffers** - Efficient event transfer
3. **Limit map sizes** - Prevent memory exhaustion
4. **Handle errors gracefully** - Both in eBPF and WASM
5. **Test on target kernel** - eBPF behavior varies by kernel version

## License

All example plugins are licensed under the Apache 2.0 License.
