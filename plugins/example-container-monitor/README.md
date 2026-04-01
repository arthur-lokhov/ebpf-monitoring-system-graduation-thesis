# Container Monitor Plugin

Monitors Docker container lifecycle events using eBPF and processes them with WASM.

## Features

- Track container starts (via execve tracepoint)
- Track container stops (via kill tracepoint)
- Monitor network connections from containers
- Real-time metrics emission

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `container_starts_total` | Counter | `container_id`, `image`, `command` | Total number of container starts |
| `container_stops_total` | Counter | `container_id`, `signal` | Total number of container stops |
| `network_connections_total` | Counter | `container_id`, `dest_ip`, `dest_port` | Network connections from containers |
| `active_containers` | Gauge | - | Current number of active containers |

## Saved Filters

- `starts_per_minute` - Container starts per minute
- `connections_per_second` - Network connections rate
- `active_containers_avg` - Average active containers

## eBPF Programs

| Program | Type | Attachment Point |
|---------|------|------------------|
| `trace_container_start` | Tracepoint | `syscalls/sys_enter_execve` |
| `trace_container_stop` | Tracepoint | `syscalls/sys_enter_kill` |
| `trace_network_connect` | Tracepoint | `syscalls/sys_enter_connect` |

## Building

```bash
./build.sh
```

Or with Docker:

```bash
./build-docker.sh
```

## Installing

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file://'"$(pwd)"'"}'
```

## Example Queries

```promql
# Container starts per minute
per_minute(container_starts_total)

# Network connections per second
rate(network_connections_total[1m])

# Active containers average
avg(active_containers)
```

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   eBPF Kernel   │────▶│   WASM User     │────▶│   Prometheus    │
│   - execve      │     │   - Event proc  │     │   Metrics       │
│   - kill        │     │   - Metrics     │     │                 │
│   - connect     │     │   - Aggregation │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## Troubleshooting

### No metrics appearing

1. Check if plugin is enabled:
   ```bash
   curl http://localhost:8080/api/v1/plugins
   ```

2. Check eBPF programs loaded:
   ```bash
   bpftool prog list
   ```

3. Check ring buffer events:
   ```bash
   bpftool map dump id <map_id>
   ```

### High memory usage

- Reduce `MAX_CONTAINERS` in WASM code
- Decrease ring buffer size in eBPF

## License

Apache 2.0
