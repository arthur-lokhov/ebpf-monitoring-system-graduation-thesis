// eBPF program for process monitoring
// Uses syscall tracepoints (compatible with WSL2)

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

// Event structure - MUST match Go EBPFEvent in loader.go
struct process_event {
    __u64 timestamp;
    __u32 pid;
    char comm[16];
    char type;  // 1=exec, 2=exit
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} process_events SEC(".maps");

// Track active processes for gauge
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u32);  // pid
    __type(value, __u64); // start timestamp
} active_processes SEC(".maps");

// Submit event to ring buffer
static __always_inline void submit_event(struct process_event *e) {
    struct process_event *event;
    event = bpf_ringbuf_reserve(&process_events, sizeof(*event), 0);
    if (event) {
        __builtin_memcpy(event, e, sizeof(*event));
        bpf_ringbuf_submit(event, 0);
    }
}

// Trace execve (process start)
SEC("tracepoint/syscalls/sys_enter_execve")
int trace_process_exec(struct bpf_tracepoint *ctx) {
    struct process_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.type = 1;  // exec

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e.pid = pid_tgid >> 32;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Track active process
    __u64 ts = e.timestamp;
    bpf_map_update_elem(&active_processes, &e.pid, &ts, BPF_ANY);

    submit_event(&e);
    return 0;
}

// Trace kill (process exit)
SEC("tracepoint/syscalls/sys_enter_kill")
int trace_process_exit(struct bpf_tracepoint *ctx) {
    struct process_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.type = 2;  // exit

    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    e.pid = pid;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Remove from active processes
    bpf_map_delete_elem(&active_processes, &pid);

    submit_event(&e);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
