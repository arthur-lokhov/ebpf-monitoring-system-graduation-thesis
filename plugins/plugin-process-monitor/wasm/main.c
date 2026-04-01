// WASM plugin for process monitoring
// Processes lifecycle events and emits Prometheus metrics

#include <stdint.h>
#include <stdbool.h>

// ============================================================================
// External functions provided by the host (Go runtime)
// ============================================================================

extern void epbf_log(int level, const char* msg, int len);
extern uint64_t epbf_now_ns(void);
extern void epbf_emit_counter(const char* name, int name_len, uint64_t value,
                               const char* labels, int labels_len);
extern void epbf_emit_gauge(const char* name, int name_len, double value,
                             const char* labels, int labels_len);
extern void epbf_emit_histogram(const char* name, int name_len, double value,
                                 const char* labels, int labels_len);
extern int epbf_subscribe_map(const char* name, int len);

// ============================================================================
// Constants and Types
// ============================================================================

#define LOG_INFO 1
#define LOG_DEBUG 2

#define EVENT_TYPE_FORK       1
#define EVENT_TYPE_EXIT       2
#define EVENT_TYPE_SCHED_SWITCH 3

#define MAX_PROCESSES 2048
#define HISTOGRAM_BUCKETS 7

struct process_event {
    uint64_t timestamp;
    uint32_t pid;
    uint32_t ppid;
    uint32_t exit_code;
    uint64_t duration_ns;
    char comm[16];
    char parent_comm[16];
    char type;
};

struct process_info {
    uint32_t pid;
    uint32_t ppid;
    uint64_t start_time;
    char comm[16];
    bool active;
};

// Histogram buckets for process duration (in seconds)
static const double duration_buckets[HISTOGRAM_BUCKETS] = {0.001, 0.01, 0.1, 1.0, 10.0, 60.0, 300.0};

// ============================================================================
// Global State
// ============================================================================

static struct process_info processes[MAX_PROCESSES];
static uint64_t duration_histogram[HISTOGRAM_BUCKETS + 1];  // +1 for +Inf
static uint64_t process_starts_total = 0;
static uint64_t process_exits_total = 0;
static uint64_t context_switches_total = 0;
static uint64_t total_cpu_time_ns = 0;

// ============================================================================
// Helper Functions
// ============================================================================

static void log_info(const char* msg) {
    epbf_log(LOG_INFO, msg, 0);
}

static int find_process(uint32_t pid) {
    for (int i = 0; i < MAX_PROCESSES; i++) {
        if (processes[i].pid == pid && processes[i].active) {
            return i;
        }
    }
    return -1;
}

static int find_or_create_process(uint32_t pid, uint32_t ppid, uint64_t timestamp, const char* comm) {
    int idx = find_process(pid);
    if (idx >= 0) {
        return idx;
    }

    for (int i = 0; i < MAX_PROCESSES; i++) {
        if (!processes[i].active) {
            processes[i].pid = pid;
            processes[i].ppid = ppid;
            processes[i].start_time = timestamp;
            for (int j = 0; j < 15 && comm[j]; j++) {
                processes[i].comm[j] = comm[j];
            }
            processes[i].comm[15] = '\0';
            processes[i].active = true;
            return i;
        }
    }
    return -1;
}

static void emit_counter_with_labels(const char* name, uint64_t value,
                                      const char* label1, const char* value1,
                                      const char* label2, const char* value2,
                                      const char* label3, const char* value3) {
    char labels[256] = {0};
    int pos = 0;

    if (label1 && value1) {
        pos += __builtin_sprintf(labels + pos, "%s=%s", label1, value1);
    }
    if (label2 && value2) {
        pos += __builtin_sprintf(labels + pos, ",%s=%s", label2, value2);
    }
    if (label3 && value3) {
        pos += __builtin_sprintf(labels + pos, ",%s=%s", label3, value3);
    }

    epbf_emit_counter(name, __builtin_strlen(name), value, labels, pos);
}

static void emit_gauge_with_labels(const char* name, double value,
                                    const char* label1, const char* value1) {
    char labels[128] = {0};
    int pos = 0;

    if (label1 && value1) {
        pos += __builtin_sprintf(labels + pos, "%s=%s", label1, value1);
    }

    epbf_emit_gauge(name, __builtin_strlen(name), value, labels, pos);
}

static void emit_histogram_with_labels(const char* name, double value,
                                        const char* label1, const char* value1) {
    char labels[128] = {0};
    int pos = 0;

    if (label1 && value1) {
        pos += __builtin_sprintf(labels + pos, "%s=%s", label1, value1);
    }

    epbf_emit_histogram(name, __builtin_strlen(name), value, labels, pos);
}

static int get_duration_bucket(uint64_t duration_ns) {
    double duration_sec = (double)duration_ns / 1000000000.0;
    
    for (int i = 0; i < HISTOGRAM_BUCKETS; i++) {
        if (duration_sec <= duration_buckets[i]) {
            return i;
        }
    }
    return HISTOGRAM_BUCKETS;  // +Inf bucket
}

// ============================================================================
// Event Processing
// ============================================================================

static void process_fork(struct process_event* e) {
    int idx = find_or_create_process(e->pid, e->ppid, e->timestamp, e->comm);
    if (idx < 0) {
        log_info("Process table full");
        return;
    }

    process_starts_total++;

    char ppid_str[16];
    __builtin_sprintf(ppid_str, "%d", e->ppid);

    // Emit counter metric
    emit_counter_with_labels(
        "process_starts_total",
        1,
        "comm", e->comm,
        "ppid", ppid_str,
        NULL, NULL
    );

    // Update active processes gauge
    int active_count = 0;
    for (int i = 0; i < MAX_PROCESSES; i++) {
        if (processes[i].active) active_count++;
    }

    emit_gauge_with_labels("active_processes", (double)active_count, NULL, NULL);

    log_info("Process started");
}

static void process_exit(struct process_event* e) {
    int idx = find_process(e->pid);
    if (idx < 0) {
        return;
    }

    process_exits_total++;

    // Update histogram
    int bucket = get_duration_bucket(e->duration_ns);
    duration_histogram[bucket]++;

    char exit_code_str[16];
    __builtin_sprintf(exit_code_str, "%d", e->exit_code);

    // Emit counter metric
    emit_counter_with_labels(
        "process_exits_total",
        1,
        "comm", processes[idx].comm,
        "exit_code", exit_code_str,
        NULL, NULL
    );

    // Emit histogram observation
    double duration_sec = (double)e->duration_ns / 1000000000.0;
    emit_histogram_with_labels(
        "process_duration_seconds",
        duration_sec,
        "comm", processes[idx].comm
    );

    // Mark process as inactive
    processes[idx].active = false;

    // Update active processes gauge
    int active_count = 0;
    for (int i = 0; i < MAX_PROCESSES; i++) {
        if (processes[i].active) active_count++;
    }

    emit_gauge_with_labels("active_processes", (double)active_count, NULL, NULL);

    log_info("Process exited");
}

static void process_sched_switch(struct process_event* e) {
    context_switches_total++;
    total_cpu_time_ns += 1000000;  // Assume 1ms per switch (simplified)

    // Emit counter metric
    emit_counter_with_labels(
        "context_switches_total",
        1,
        "prev_comm", e->parent_comm,
        "next_comm", e->comm,
        NULL, NULL
    );

    // Emit CPU time
    emit_counter_with_labels(
        "cpu_time_seconds_total",
        1,
        "comm", e->comm,
        NULL, NULL,
        NULL, NULL
    );
}

// ============================================================================
// Exported Functions
// ============================================================================

__attribute__((export))
int epbf_init(void) {
    // Initialize processes array
    for (int i = 0; i < MAX_PROCESSES; i++) {
        processes[i].active = false;
        processes[i].pid = 0;
    }

    // Initialize histogram
    for (int i = 0; i <= HISTOGRAM_BUCKETS; i++) {
        duration_histogram[i] = 0;
    }

    // Subscribe to eBPF map
    epbf_subscribe_map("process_events", 14);

    log_info("Process monitor initialized");
    return 0;
}

__attribute__((export))
void process_events(void) {
    // In a real implementation, this would read from the eBPF ring buffer
}

__attribute__((export))
void epbf_cleanup(void) {
    log_info("Process monitor cleanup");
}

// ============================================================================
// Statistics Functions
// ============================================================================

__attribute__((export))
uint64_t get_total_process_starts(void) {
    return process_starts_total;
}

__attribute__((export))
uint64_t get_total_process_exits(void) {
    return process_exits_total;
}

__attribute__((export))
uint64_t get_total_context_switches(void) {
    return context_switches_total;
}

__attribute__((export))
int get_active_process_count(void) {
    int count = 0;
    for (int i = 0; i < MAX_PROCESSES; i++) {
        if (processes[i].active) count++;
    }
    return count;
}

__attribute__((export))
uint64_t get_histogram_bucket(int bucket) {
    if (bucket < 0 || bucket > HISTOGRAM_BUCKETS) {
        return 0;
    }
    return duration_histogram[bucket];
}

__attribute__((export))
uint64_t get_total_cpu_time_ns(void) {
    return total_cpu_time_ns;
}
