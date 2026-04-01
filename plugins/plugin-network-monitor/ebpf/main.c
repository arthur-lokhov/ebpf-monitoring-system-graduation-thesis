// eBPF program for network monitoring
// Tracks TCP connections, bytes sent/received

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

// Network event structure
struct network_event {
    __u64 timestamp;
    __u32 pid;
    __u32 dest_ip;
    __u16 dest_port;
    __u16 src_port;
    __u64 bytes;
    char type;  // 1=connect, 2=send, 3=recv
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);
} network_events SEC(".maps");

// Connection tracking
struct connection_key {
    __u32 dest_ip;
    __u16 dest_port;
};

struct connection_value {
    __u64 start_time;
    __u64 bytes_sent;
    __u64 bytes_received;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, struct connection_key);
    __type(value, struct connection_value);
} connections SEC(".maps");

// Submit event to ring buffer
static __always_inline void submit_event(struct network_event *e) {
    struct network_event *event;
    event = bpf_ringbuf_reserve(&network_events, sizeof(*event), 0);
    if (event) {
        __builtin_memcpy(event, e, sizeof(*event));
        bpf_ringbuf_submit(event, 0);
    }
}

// Trace TCP connect
SEC("tracepoint/syscalls/sys_enter_connect")
int trace_tcp_connect(struct bpf_tracepoint *ctx) {
    struct network_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 1;  // connect
    e.bytes = 0;

    // Extract destination IP and port from sockaddr_in
    // This is simplified - real implementation would parse the structure
    e.dest_ip = 0;  // Would extract from args
    e.dest_port = 0;  // Would extract from args

    // Track connection start
    struct connection_key key = {};
    key.dest_ip = e.dest_ip;
    key.dest_port = e.dest_port;

    struct connection_value val = {};
    val.start_time = e.timestamp;
    val.bytes_sent = 0;
    val.bytes_received = 0;

    bpf_map_update_elem(&connections, &key, &val, BPF_ANY);

    submit_event(&e);
    return 0;
}

// Trace TCP sendmsg
SEC("kprobe/tcp_sendmsg")
int trace_tcp_sendmsg(struct pt_regs *ctx) {
    struct network_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 2;  // send

    // Get size from args
    e.bytes = PT_REGS_PARM3(ctx);

    // Get socket info (simplified)
    e.dest_ip = 0;  // Would extract from socket
    e.dest_port = 0;  // Would extract from socket

    // Update bytes sent
    struct connection_key key = {};
    key.dest_ip = e.dest_ip;
    key.dest_port = e.dest_port;

    struct connection_value *val = bpf_map_lookup_elem(&connections, &key);
    if (val) {
        val->bytes_sent += e.bytes;
        bpf_map_update_elem(&connections, &key, val, BPF_EXIST);
    }

    submit_event(&e);
    return 0;
}

// Trace TCP recvmsg
SEC("kprobe/tcp_recvmsg")
int trace_tcp_recvmsg(struct pt_regs *ctx) {
    struct network_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 3;  // recv

    // Size will be known on return (kretprobe would be better)
    e.bytes = 0;
    e.dest_ip = 0;
    e.dest_port = 0;

    submit_event(&e);
    return 0;
}

// Calculate connection duration and emit histogram
static __always_inline void record_connection_duration(
    struct connection_key *key,
    struct connection_value *val,
    __u64 end_time
) {
    __u64 duration_ns = end_time - val->start_time;
    __u64 duration_sec = duration_ns / 1000000000;

    // Simple bucketing
    int bucket = 0;
    if (duration_sec > 60) bucket = 4;      // > 60s
    else if (duration_sec > 10) bucket = 3; // > 10s
    else if (duration_sec > 1) bucket = 2;  // > 1s
    else if (duration_sec > 0.1) bucket = 1; // > 0.1s
    else bucket = 0;                         // <= 0.1s

    // Would emit histogram event here
}

char LICENSE[] SEC("license") = "GPL";
