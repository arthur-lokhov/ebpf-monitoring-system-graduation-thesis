// eBPF program for disk I/O monitoring
// Tracks read/write operations, bytes transferred, and I/O latency

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

// Disk I/O event structure
struct disk_event {
    __u64 timestamp;
    __u32 pid;
    __u32 dev;
    __u64 bytes;
    __u64 latency_ns;
    char type;      // 1=read, 2=write
    char comm[16];
    char filename[256];
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);
} disk_events SEC(".maps");

// Track start time for latency calculation
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u64);  // pid + sector
    __type(value, __u64); // start timestamp
} io_start SEC(".maps");

// Device statistics
struct dev_stats {
    __u64 read_bytes;
    __u64 write_bytes;
    __u64 read_ops;
    __u64 write_ops;
    __u64 io_time_ns;
    __u64 queue_length;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 64);
    __type(key, __u32);  // device
    __type(value, struct dev_stats);
} dev_stats_map SEC(".maps");

// Submit event to ring buffer
static __always_inline void submit_event(struct disk_event *e) {
    struct disk_event *event;
    event = bpf_ringbuf_reserve(&disk_events, sizeof(*event), 0);
    if (event) {
        __builtin_memcpy(event, e, sizeof(*event));
        bpf_ringbuf_submit(event, 0);
    }
}

// Update device stats
static __always_inline void update_dev_stats(__u32 dev, bool is_read, __u64 bytes, __u64 latency) {
    struct dev_stats *stats = bpf_map_lookup_elem(&dev_stats_map, &dev);
    if (!stats) {
        struct dev_stats new_stats = {};
        bpf_map_update_elem(&dev_stats_map, &dev, &new_stats, BPF_ANY);
        stats = bpf_map_lookup_elem(&dev_stats_map, &dev);
        if (!stats) return;
    }

    if (is_read) {
        stats->read_bytes += bytes;
        stats->read_ops++;
    } else {
        stats->write_bytes += bytes;
        stats->write_ops++;
    }

    stats->io_time_ns += latency;
}

// Trace VFS read entry
SEC("kprobe/vfs_read")
int trace_vfs_read_entry(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 timestamp = bpf_ktime_get_ns();

    // Store start time for latency calculation
    bpf_map_update_elem(&io_start, &pid_tgid, &timestamp, BPF_ANY);

    return 0;
}

// Trace VFS read return
SEC("kretprobe/vfs_read")
int trace_vfs_read_return(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 end_time = bpf_ktime_get_ns();

    // Get start time
    __u64 *start_time = bpf_map_lookup_elem(&io_start, &pid_tgid);
    if (!start_time) {
        return 0;
    }

    __u64 latency = end_time - *start_time;
    bpf_map_delete_elem(&io_start, &pid_tgid);

    // Get return value (bytes read)
    __u64 bytes = PT_REGS_RC(ctx);
    if (bytes <= 0) {
        return 0;
    }

    // Create event
    struct disk_event e = {};
    e.timestamp = end_time;
    e.pid = pid_tgid >> 32;
    e.type = 1;  // read
    e.bytes = bytes;
    e.latency_ns = latency;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Get device info (simplified - would need to extract from file struct)
    e.dev = 0;

    submit_event(&e);

    // Update stats
    update_dev_stats(e.dev, true, bytes, latency);

    return 0;
}

// Trace VFS write entry
SEC("kprobe/vfs_write")
int trace_vfs_write_entry(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 timestamp = bpf_ktime_get_ns();

    // Store start time
    bpf_map_update_elem(&io_start, &pid_tgid, &timestamp, BPF_ANY);

    return 0;
}

// Trace VFS write return
SEC("kretprobe/vfs_write")
int trace_vfs_write_return(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 end_time = bpf_ktime_get_ns();

    // Get start time
    __u64 *start_time = bpf_map_lookup_elem(&io_start, &pid_tgid);
    if (!start_time) {
        return 0;
    }

    __u64 latency = end_time - *start_time;
    bpf_map_delete_elem(&io_start, &pid_tgid);

    // Get bytes written
    __u64 bytes = PT_REGS_RC(ctx);
    if (bytes <= 0) {
        return 0;
    }

    // Create event
    struct disk_event e = {};
    e.timestamp = end_time;
    e.pid = pid_tgid >> 32;
    e.type = 2;  // write
    e.bytes = bytes;
    e.latency_ns = latency;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    e.dev = 0;  // Would extract from file struct

    submit_event(&e);

    // Update stats
    update_dev_stats(e.dev, false, bytes, latency);

    return 0;
}

// Trace block layer request
SEC("tracepoint/block/block_rq_issue")
int trace_block_rq_issue(struct bpf_tracepoint *ctx) {
    struct disk_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;

    // Extract device and bytes from tracepoint args
    // e.dev = ctx->dev;
    // e.bytes = ctx->bytes;

    submit_event(&e);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
