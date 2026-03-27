// eBPF program for container monitoring (minimal version)
// Uses BPF_PROG_TYPE_TRACEPOINT

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

// Event structure
struct container_event {
    __u64 timestamp;
    __u32 pid;
    char comm[16];
    char type;  // 1=start, 2=stop, 3=network
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} container_events SEC(".maps");

// Active containers tracking
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u32);
    __type(value, __u64);
} active_containers SEC(".maps");

// Submit event to ring buffer
static __always_inline void submit_event(struct container_event *e) {
    struct container_event *event;
    event = bpf_ringbuf_reserve(&container_events, sizeof(*event), 0);
    if (event) {
        __builtin_memcpy(event, e, sizeof(*event));
        bpf_ringbuf_submit(event, 0);
    }
}

// Trace execve (container start)
SEC("tracepoint/syscalls/sys_enter_execve")
int trace_container_start(struct bpf_tracepoint *ctx) {
    struct container_event e = {};
    
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 1;
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    // Track active container
    __u64 ts = e.timestamp;
    bpf_map_update_elem(&active_containers, &e.pid, &ts, BPF_ANY);
    
    submit_event(&e);
    return 0;
}

// Trace kill (container stop)
SEC("tracepoint/syscalls/sys_enter_kill")
int trace_container_stop(struct bpf_tracepoint *ctx) {
    struct container_event e = {};
    
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 2;
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    // Remove from active
    bpf_map_delete_elem(&active_containers, &e.pid);
    
    submit_event(&e);
    return 0;
}

// Trace connect (network)
SEC("tracepoint/syscalls/sys_enter_connect")
int trace_network_connect(struct bpf_tracepoint *ctx) {
    struct container_event e = {};
    
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 3;
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    submit_event(&e);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
