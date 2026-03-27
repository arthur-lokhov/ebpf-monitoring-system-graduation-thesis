// WASM plugin for container monitoring
// Processes eBPF events and emits metrics

#include "../../pkg/wasmsdk/include/epbf.h"
#include <stdint.h>
#include <stdbool.h>

// Event types
#define EVENT_START    1
#define EVENT_STOP     2
#define EVENT_NETWORK  3

// Container event structure (must match eBPF)
struct container_event {
    uint64_t timestamp;
    uint32_t pid;
    uint32_t ppid;
    char comm[16];
    char filename[256];
    char type;
};

// Active container tracking
typedef struct {
    char container_id[64];
    char image[128];
    uint64_t start_time;
    bool active;
} container_info_t;

static container_info_t containers[100];
static int container_count = 0;

// Find or create container entry
static container_info_t* get_container(const char *comm) {
    // Search existing
    for (int i = 0; i < container_count; i++) {
        if (__builtin_strcmp(containers[i].comm, comm) == 0) {
            return &containers[i];
        }
    }
    
    // Create new
    if (container_count < 100) {
        container_info_t *c = &containers[container_count++];
        __builtin_strncpy(c->container_id, comm, 64);
        __builtin_strncpy(c->image, comm, 128);
        c->start_time = epbf_now_ns();
        c->active = true;
        return c;
    }
    
    return 0;
}

// Process container start event
static void process_start(struct container_event *e) {
    container_info_t *c = get_container(e->comm);
    if (c) {
        c->active = true;
        c->start_time = e->timestamp;
        
        // Emit counter metric
        epbf_label_t labels[] = {
            {"container_id", c->container_id},
            {"image", c->image},
            {"command", e->filename}
        };
        epbf_emit_counter("container_starts_total", 1, labels, 3);
        
        EPBF_LOG_INFO("Container started: %s (image: %s)", c->container_id, c->image);
    }
}

// Process container stop event
static void process_stop(struct container_event *e) {
    container_info_t *c = get_container(e->comm);
    if (c) {
        c->active = false;
        
        // Extract signal from filename field
        char signal[8];
        signal[0] = e->filename[0];
        signal[1] = '\0';
        
        // Emit counter metric
        epbf_label_t labels[] = {
            {"container_id", c->container_id},
            {"signal", signal}
        };
        epbf_emit_counter("container_stops_total", 1, labels, 2);
        
        EPBF_LOG_INFO("Container stopped: %s (signal: %s)", c->container_id, signal);
    }
}

// Process network connection event
static void process_network(struct container_event *e) {
    container_info_t *c = get_container(e->comm);
    if (c) {
        // Emit counter metric
        epbf_label_t labels[] = {
            {"container_id", c->container_id},
            {"dest_ip", "0.0.0.0"},  // Would be parsed from actual event
            {"dest_port", "0"}
        };
        epbf_emit_counter("network_connections_total", 1, labels, 3);
        
        EPBF_LOG_DEBUG("Network connection from: %s", c->container_id);
    }
}

// Update active containers gauge
static void update_active_gauge() {
    int active = 0;
    for (int i = 0; i < container_count; i++) {
        if (containers[i].active) {
            active++;
        }
    }
    
    epbf_label_t labels[] = {};
    epbf_emit_gauge("active_containers", (double)active, labels, 0);
}

// Plugin initialization
int epbf_init(void) {
    EPBF_LOG_INFO("Container monitor plugin initialized");
    EPBF_LOG_INFO("Monitoring container events...");
    
    // Subscribe to eBPF map
    int map_fd = epbf_subscribe_map("container_events");
    if (map_fd < 0) {
        EPBF_LOG_ERROR("Failed to subscribe to container_events map");
        return -1;
    }
    
    EPBF_LOG_INFO("Subscribed to container_events map (fd=%d)", map_fd);
    
    return 0;
}

// Main event loop (called by runtime)
void process_events(void) {
    // This would be called by the WASM runtime when events are available
    // For now, just update the gauge periodically
    update_active_gauge();
}

// Cleanup
void epbf_cleanup(void) {
    EPBF_LOG_INFO("Container monitor plugin cleanup");
}
