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

// Check if process is running inside a container
static __always_inline int is_container_process(void) {
    // Get cgroup ID
    __u64 cgroup_id = bpf_get_current_cgroup_id();
    if (cgroup_id == 0) {
        return 0; // Not in cgroup = not a container
    }
    
    // Check if cgroup path contains docker/containerd markers
    // This is a simplified check - in production you'd parse cgroup path
    // For now, we check if cgroup_id is non-zero (means it's in a cgroup namespace)
    // Most container runtimes create dedicated cgroups
    
    // Additional check: container processes typically have non-init PID in their namespace
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 tgid = pid_tgid >> 32;
    __u32 pid = pid_tgid;
    
    // If thread ID != process ID, it's a thread, skip
    if (tgid != pid) {
        return 0;
    }
    
    // If PID is very low (< 10), likely system process, skip
    if (pid < 10) {
        return 0;
    }
    
    return 1;
}

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
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    // Only track if this PID is NOT already in active_containers
    // This ensures we only count the FIRST execve per process
    __u64 *existing = bpf_map_lookup_elem(&active_containers, &pid);
    if (existing) {
        return 0; // Already tracked, skip
    }
    
    // Check if this is a container process
    __u64 cgroup_id = bpf_get_current_cgroup_id();
    if (cgroup_id == 0) {
        return 0; // Not in cgroup
    }
    
    struct container_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = pid;
    e.type = 1;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Track active container
    __u64 ts = e.timestamp;
    bpf_map_update_elem(&active_containers, &pid, &ts, BPF_ANY);

    submit_event(&e);
    return 0;
}

// Trace kill (container stop)
SEC("tracepoint/syscalls/sys_enter_kill")
int trace_container_stop(struct bpf_tracepoint *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    // Only track if this PID was previously seen as container
    __u64 *start_time = bpf_map_lookup_elem(&active_containers, &pid);
    if (!start_time) {
        return 0; // Not a tracked container
    }
    
    struct container_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = pid;
    e.type = 2;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Remove from active
    bpf_map_delete_elem(&active_containers, &pid);

    submit_event(&e);
    return 0;
}

// Trace connect (network)
SEC("tracepoint/syscalls/sys_enter_connect")
int trace_network_connect(struct bpf_tracepoint *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    
    // Only track if this PID was previously seen as container
    __u64 *start_time = bpf_map_lookup_elem(&active_containers, &pid);
    if (!start_time) {
        return 0; // Not a tracked container
    }
    
    struct container_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.pid = pid;
    e.type = 3;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    submit_event(&e);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
