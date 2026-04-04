// WASM plugin for container monitoring
// Processes eBPF events and emits Prometheus metrics

#include <stdint.h>
#include <stdbool.h>
#include "../../pkg/wasmsdk/include/epbf_imports.h"

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
    // For demo, we emit test metrics on each call
    demo_emit_metrics();
}

__attribute__((export))
void epbf_cleanup(void) {
    log_info("Container monitor cleanup");
}

// ============================================================================
// Demo Mode - Generate test metrics without eBPF
// ============================================================================

static uint64_t demo_counter = 0;
static uint64_t demo_active = 0;

// Metric type constants - host will recognize these
#define METRIC_STARTS     1
#define METRIC_STOPS      2
#define METRIC_NETWORK    3
#define METRIC_ACTIVE     4

__attribute__((export))
void demo_emit_metrics(void) {
    demo_counter++;
    demo_active = (demo_counter % 10);

    // Emit demo metrics - using volatile to prevent optimization
    volatile uint64_t counter = demo_counter;
    volatile uint64_t active = demo_active;
    
    // Call emit functions with actual string data
    static const char starts[] = "container_starts_total";
    static const char stops[] = "container_stops_total";
    static const char network[] = "network_connections_total";
    static const char active_name[] = "active_containers";
    
    epbf_emit_counter(starts, sizeof(starts)-1, counter, 0, 0);
    epbf_emit_counter(stops, sizeof(stops)-1, counter / 2, 0, 0);
    epbf_emit_counter(network, sizeof(network)-1, counter * 3, 0, 0);
    epbf_emit_gauge(active_name, sizeof(active_name)-1, (double)active, 0, 0);

    log_info("Demo metrics emitted");
}
