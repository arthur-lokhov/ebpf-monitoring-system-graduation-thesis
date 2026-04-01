// WASM plugin for container monitoring
// Processes eBPF events and emits Prometheus metrics

#include <stdint.h>
#include <stdbool.h>

// ============================================================================
// External functions provided by the host (Go runtime)
// ============================================================================

// Logging
extern void epbf_log(int level, const char* msg, int len);

// Time
extern uint64_t epbf_now_ns(void);

// Metric emission
extern void epbf_emit_counter(const char* name, int name_len, uint64_t value,
                               const char* labels, int labels_len);
extern void epbf_emit_gauge(const char* name, int name_len, double value,
                             const char* labels, int labels_len);

// eBPF map access (stub for now)
extern int epbf_subscribe_map(const char* name, int len);
extern int epbf_read_map(const char* name, int name_len,
                         const void* key, int key_size,
                         void* value, int value_size);

// ============================================================================
// Constants and Types
// ============================================================================

#define LOG_INFO 1
#define LOG_DEBUG 2

#define EVENT_TYPE_START    1
#define EVENT_TYPE_STOP     2
#define EVENT_TYPE_NETWORK  3

#define MAX_CONTAINERS 256
#define MAX_EVENT_QUEUE 64

// Event from eBPF
struct container_event {
    uint64_t timestamp;
    uint32_t pid;
    char comm[16];
    char type;
};

// Tracked container
struct container_info {
    uint32_t pid;
    char comm[16];
    uint64_t start_time;
    uint64_t event_count;
    bool active;
};

// ============================================================================
// Global State
// ============================================================================

static struct container_info containers[MAX_CONTAINERS];
static struct container_event event_queue[MAX_EVENT_QUEUE];
static int event_queue_head = 0;
static int event_queue_tail = 0;
static uint64_t total_starts = 0;
static uint64_t total_stops = 0;
static uint64_t total_connections = 0;

// ============================================================================
// Helper Functions
// ============================================================================

static void log_info(const char* msg) {
    epbf_log(LOG_INFO, msg, 0);
}

static int find_container(uint32_t pid) {
    for (int i = 0; i < MAX_CONTAINERS; i++) {
        if (containers[i].pid == pid && containers[i].active) {
            return i;
        }
    }
    return -1;
}

static int find_free_container_slot(void) {
    for (int i = 0; i < MAX_CONTAINERS; i++) {
        if (!containers[i].active) {
            return i;
        }
    }
    return -1;
}

static void emit_counter_with_labels(const char* name, uint64_t value,
                                      const char* label1, const char* value1,
                                      const char* label2, const char* value2,
                                      const char* label3, const char* value3) {
    // Simple label encoding: "label1=value1,label2=value2,label3=value3"
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

// ============================================================================
// Event Processing
// ============================================================================

static void process_container_start(struct container_event* e) {
    int slot = find_free_container_slot();
    if (slot < 0) {
        log_info("Container slot full");
        return;
    }
    
    containers[slot].pid = e->pid;
    containers[slot].start_time = e->timestamp;
    containers[slot].event_count = 1;
    containers[slot].active = true;
    
    // Copy comm
    for (int i = 0; i < 16 && e->comm[i]; i++) {
        containers[slot].comm[i] = e->comm[i];
    }
    containers[slot].comm[15] = '\0';
    
    total_starts++;
    
    // Emit metrics
    emit_counter_with_labels(
        "container_starts_total",
        1,
        "container_id", containers[slot].comm,
        "image", "unknown",
        "command", containers[slot].comm
    );
    
    // Update active containers gauge
    int active_count = 0;
    for (int i = 0; i < MAX_CONTAINERS; i++) {
        if (containers[i].active) active_count++;
    }
    
    emit_gauge_with_labels("active_containers", (double)active_count, NULL, NULL);
    
    log_info("Container started");
}

static void process_container_stop(struct container_event* e) {
    int slot = find_container(e->pid);
    if (slot < 0) {
        return;
    }
    
    total_stops++;
    
    // Emit metric
    emit_counter_with_labels(
        "container_stops_total",
        1,
        "container_id", containers[slot].comm,
        "signal", "unknown",
        NULL, NULL
    );
    
    containers[slot].active = false;
    
    // Update active containers gauge
    int active_count = 0;
    for (int i = 0; i < MAX_CONTAINERS; i++) {
        if (containers[i].active) active_count++;
    }
    
    emit_gauge_with_labels("active_containers", (double)active_count, NULL, NULL);
    
    log_info("Container stopped");
}

static void process_network_connect(struct container_event* e) {
    int slot = find_container(e->pid);
    if (slot < 0) {
        // Network event from unknown container, use comm anyway
        total_connections++;
        emit_counter_with_labels(
            "network_connections_total",
            1,
            "container_id", e->comm,
            "dest_ip", "0.0.0.0",
            "dest_port", "0"
        );
        return;
    }
    
    containers[slot].event_count++;
    total_connections++;
    
    // Emit metric
    emit_counter_with_labels(
        "network_connections_total",
        1,
        "container_id", containers[slot].comm,
        "dest_ip", "0.0.0.0",
        "dest_port", "0"
    );
    
    log_info("Network connection");
}

// ============================================================================
// Exported Functions
// ============================================================================

__attribute__((export))
int epbf_init(void) {
    // Initialize containers array
    for (int i = 0; i < MAX_CONTAINERS; i++) {
        containers[i].active = false;
        containers[i].pid = 0;
        containers[i].start_time = 0;
        containers[i].event_count = 0;
    }
    
    // Subscribe to eBPF map
    epbf_subscribe_map("container_events", 16);
    
    log_info("Container monitor initialized");
    return 0;
}

__attribute__((export))
void process_events(void) {
    // In a real implementation, this would read from the eBPF ring buffer
    // For now, we'll just process the event queue
    
    while (event_queue_head != event_queue_tail) {
        struct container_event e = event_queue[event_queue_head];
        event_queue_head = (event_queue_head + 1) % MAX_EVENT_QUEUE;
        
        switch (e.type) {
            case EVENT_TYPE_START:
                process_container_start(&e);
                break;
            case EVENT_TYPE_STOP:
                process_container_stop(&e);
                break;
            case EVENT_TYPE_NETWORK:
                process_network_connect(&e);
                break;
        }
    }
}

__attribute__((export))
void epbf_cleanup(void) {
    log_info("Container monitor cleanup");
}

// ============================================================================
// Test/Debug Functions (for development)
// ============================================================================

// Simulate container start event
__attribute__((export))
void test_simulate_start(uint32_t pid, const char* comm) {
    struct container_event e = {
        .timestamp = epbf_now_ns(),
        .pid = pid,
        .type = EVENT_TYPE_START,
    };
    
    for (int i = 0; i < 15 && comm[i]; i++) {
        e.comm[i] = comm[i];
    }
    
    int slot = (event_queue_tail + 1) % MAX_EVENT_QUEUE;
    if (slot == event_queue_head) {
        log_info("Event queue full");
        return;
    }
    
    event_queue[event_queue_tail] = e;
    event_queue_tail = slot;
}

// Simulate container stop event
__attribute__((export))
void test_simulate_stop(uint32_t pid) {
    struct container_event e = {
        .timestamp = epbf_now_ns(),
        .pid = pid,
        .type = EVENT_TYPE_STOP,
    };
    
    int slot = (event_queue_tail + 1) % MAX_EVENT_QUEUE;
    if (slot == event_queue_head) {
        log_info("Event queue full");
        return;
    }
    
    event_queue[event_queue_tail] = e;
    event_queue_tail = slot;
}

// Get stats
__attribute__((export))
uint64_t test_get_total_starts(void) {
    return total_starts;
}

__attribute__((export))
uint64_t test_get_total_stops(void) {
    return total_stops;
}

__attribute__((export))
uint64_t test_get_total_connections(void) {
    return total_connections;
}
