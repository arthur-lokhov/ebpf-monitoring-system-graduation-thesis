// eBPF program for container monitoring
// Tracks container start/stop events and network connections

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <linux/sched.h>

// Event structures
struct container_event {
    __u64 timestamp;
    __u32 pid;
    __u32 ppid;
    char comm[TASK_COMM_LEN];
    char filename[256];
    char type;  // 1=start, 2=stop, 3=network
};

// Map for storing events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} container_events SEC(".maps");

// Map for tracking active containers
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u32);  // pid
    __type(value, __u64);  // start timestamp
} active_containers SEC(".maps");

// Map for metrics - container starts
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct container_event);
    __type(value, __u64);  // count
} container_starts SEC(".maps");

// Map for metrics - network connections
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct container_event);
    __type(value, __u64);  // count
} network_connections SEC(".maps");

// Helper to submit events
static __always_inline void submit_event(struct container_event *e) {
    struct container_event *event;
    event = bpf_ringbuf_reserve(&container_events, sizeof(*event), 0);
    if (event) {
        event->timestamp = e->timestamp;
        event->pid = e->pid;
        event->ppid = e->ppid;
        event->type = e->type;
        __builtin_memcpy(event->comm, e->comm, TASK_COMM_LEN);
        __builtin_memcpy(event->filename, e->filename, sizeof(e->filename));
        bpf_ringbuf_submit(event, 0);
    }
}

// Trace container start (execve)
SEC("tracepoint/syscalls/sys_enter_execve")
int trace_container_start(struct trace_event_raw_sys_enter *ctx)
{
    struct container_event e = {};
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.ppid = bpf_get_current_pid_tgid() & 0xFFFFFFFF;
    e.type = 1;  // start
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    // Get filename (container image/command)
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(&e.filename, sizeof(e.filename), filename);
    
    // Track in active containers map
    __u64 timestamp = e.timestamp;
    bpf_map_update_elem(&active_containers, &e.pid, &timestamp, BPF_ANY);
    
    // Submit event
    submit_event(&e);
    
    return 0;
}

// Trace container stop (kill signal)
SEC("tracepoint/syscalls/sys_enter_kill")
int trace_container_stop(struct trace_event_raw_sys_enter *ctx)
{
    struct container_event e = {};
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 2;  // stop
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    // Get signal number
    __u32 sig = (__u32)ctx->args[1];
    e.filename[0] = sig;  // Store signal in filename field
    
    // Remove from active containers
    bpf_map_delete_elem(&active_containers, &e.pid);
    
    // Submit event
    submit_event(&e);
    
    return 0;
}

// Trace network connections
SEC("tracepoint/syscalls/sys_enter_connect")
int trace_network_connect(struct trace_event_raw_sys_enter *ctx)
{
    struct container_event e = {};
    e.timestamp = bpf_ktime_get_ns();
    e.pid = bpf_get_current_pid_tgid() >> 32;
    e.type = 3;  // network
    
    bpf_get_current_comm(&e.comm, sizeof(e.comm));
    
    // Submit event
    submit_event(&e);
    
    return 0;
}

// License
char LICENSE[] SEC("license") = "GPL";
