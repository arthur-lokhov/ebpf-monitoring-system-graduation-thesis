// eBPF program for process monitoring
// Tracks process lifecycle, CPU time, and context switches

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

// Process event structure
struct process_event {
    __u64 timestamp;
    __u32 pid;
    __u32 ppid;
    __u32 exit_code;
    __u64 duration_ns;
    char comm[16];
    char parent_comm[16];
    char type;  // 1=fork, 2=exit, 3=sched_switch
};

// Ring buffer for events
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 512 * 1024);
} process_events SEC(".maps");

// Track process start times for duration calculation
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u32);  // pid
    __type(value, __u64); // start timestamp
} process_start SEC(".maps");

// Process statistics
struct process_stats {
    __u64 start_count;
    __u64 exit_count;
    __u64 total_cpu_time_ns;
    __u64 total_duration_ns;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, char[16]);  // comm
    __type(value, struct process_stats);
} process_stats_map SEC(".maps");

// Submit event to ring buffer
static __always_inline void submit_event(struct process_event *e) {
    struct process_event *event;
    event = bpf_ringbuf_reserve(&process_events, sizeof(*event), 0);
    if (event) {
        __builtin_memcpy(event, e, sizeof(*event));
        bpf_ringbuf_submit(event, 0);
    }
}

// Update process stats
static __always_inline void update_process_stats(const char* comm, bool is_start, __u64 duration, __u64 cpu_time) {
    struct process_stats *stats = bpf_map_lookup_elem(&process_stats_map, comm);
    if (!stats) {
        struct process_stats new_stats = {};
        bpf_map_update_elem(&process_stats_map, comm, &new_stats, BPF_ANY);
        stats = bpf_map_lookup_elem(&process_stats_map, comm);
        if (!stats) return;
    }

    if (is_start) {
        stats->start_count++;
    } else {
        stats->exit_count++;
        stats->total_duration_ns += duration;
        stats->total_cpu_time_ns += cpu_time;
    }
}

// Trace process fork
SEC("tracepoint/sched/sched_process_fork")
int trace_sched_process_fork(struct bpf_tracepoint *ctx) {
    struct process_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.type = 1;  // fork

    // Extract parent and child info from tracepoint
    // Simplified - real implementation would access tracepoint fields
    e.ppid = bpf_get_current_pid_tgid() >> 32;
    e.pid = 0;  // Would extract from tracepoint

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Track start time
    bpf_map_update_elem(&process_start, &e.pid, &e.timestamp, BPF_ANY);

    // Update stats
    update_process_stats(e.comm, true, 0, 0);

    submit_event(&e);
    return 0;
}

// Trace process exit
SEC("tracepoint/sched/sched_process_exit")
int trace_sched_process_exit(struct bpf_tracepoint *ctx) {
    struct process_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.type = 2;  // exit

    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    e.pid = pid;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Get start time and calculate duration
    __u64 *start_time = bpf_map_lookup_elem(&process_start, &pid);
    if (start_time) {
        e.duration_ns = e.timestamp - *start_time;
        bpf_map_delete_elem(&process_start, &pid);
    }

    // Exit code would be extracted from tracepoint
    e.exit_code = 0;

    // Update stats
    update_process_stats(e.comm, false, e.duration_ns, 0);

    submit_event(&e);
    return 0;
}

// Trace context switch
SEC("tracepoint/sched/sched_switch")
int trace_sched_switch(struct bpf_tracepoint *ctx) {
    struct process_event e = {};

    e.timestamp = bpf_ktime_get_ns();
    e.type = 3;  // sched_switch

    // Extract prev and next task info
    // Simplified - real implementation would access tracepoint fields
    e.pid = bpf_get_current_pid_tgid() >> 32;

    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    submit_event(&e);
    return 0;
}

// Count active processes
SEC("tracepoint/sched/sched_process_free")
int trace_sched_process_free(struct bpf_tracepoint *ctx) {
    // Clean up any remaining entries
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    bpf_map_delete_elem(&process_start, &pid);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
