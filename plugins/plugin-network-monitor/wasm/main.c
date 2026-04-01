// WASM plugin for network monitoring
// Processes network events and emits Prometheus metrics

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

#define EVENT_TYPE_CONNECT  1
#define EVENT_TYPE_SEND     2
#define EVENT_TYPE_RECV     3

#define MAX_CONNECTIONS 1024

struct network_event {
    uint64_t timestamp;
    uint32_t pid;
    uint32_t dest_ip;
    uint16_t dest_port;
    uint16_t src_port;
    uint64_t bytes;
    char type;
};

struct connection {
    uint32_t dest_ip;
    uint16_t dest_port;
    uint64_t start_time;
    uint64_t bytes_sent;
    uint64_t bytes_received;
    bool active;
};

// ============================================================================
// Global State
// ============================================================================

static struct connection connections[MAX_CONNECTIONS];
static uint64_t tcp_connections_total = 0;
static uint64_t bytes_sent_total = 0;
static uint64_t bytes_received_total = 0;

// ============================================================================
// Helper Functions
// ============================================================================

static void log_info(const char* msg) {
    epbf_log(LOG_INFO, msg, 0);
}

static int find_connection(uint32_t dest_ip, uint16_t dest_port) {
    for (int i = 0; i < MAX_CONNECTIONS; i++) {
        if (connections[i].dest_ip == dest_ip &&
            connections[i].dest_port == dest_port &&
            connections[i].active) {
            return i;
        }
    }
    return -1;
}

static int find_or_create_connection(uint32_t dest_ip, uint16_t dest_port, uint64_t timestamp) {
    int idx = find_connection(dest_ip, dest_port);
    if (idx >= 0) {
        return idx;
    }

    // Find free slot
    for (int i = 0; i < MAX_CONNECTIONS; i++) {
        if (!connections[i].active) {
            connections[i].dest_ip = dest_ip;
            connections[i].dest_port = dest_port;
            connections[i].start_time = timestamp;
            connections[i].bytes_sent = 0;
            connections[i].bytes_received = 0;
            connections[i].active = true;
            return i;
        }
    }
    return -1;  // No free slot
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

static const char* ip_to_string(uint32_t ip, char* buf, int buf_size) {
    __builtin_sprintf(buf, "%d.%d.%d.%d",
                      (ip >> 24) & 0xFF,
                      (ip >> 16) & 0xFF,
                      (ip >> 8) & 0xFF,
                      ip & 0xFF);
    return buf;
}

// ============================================================================
// Event Processing
// ============================================================================

static void process_connect(struct network_event* e) {
    int idx = find_or_create_connection(e->dest_ip, e->dest_port, e->timestamp);
    if (idx < 0) {
        log_info("Connection table full");
        return;
    }

    tcp_connections_total++;

    char dest_ip_str[16];
    char dest_port_str[8];
    ip_to_string(e->dest_ip, dest_ip_str, sizeof(dest_ip_str));
    __builtin_sprintf(dest_port_str, "%d", e->dest_port);

    // Emit counter metric
    emit_counter_with_labels(
        "tcp_connections_total",
        1,
        "dest_ip", dest_ip_str,
        "dest_port", dest_port_str,
        "proto", "tcp"
    );

    // Update active connections gauge
    int active_count = 0;
    for (int i = 0; i < MAX_CONNECTIONS; i++) {
        if (connections[i].active) active_count++;
    }

    char port_str[8];
    __builtin_sprintf(port_str, "%d", e->dest_port);
    emit_gauge_with_labels("active_connections", (double)active_count, "dest_port", port_str);

    log_info("TCP connection established");
}

static void process_send(struct network_event* e) {
    int idx = find_connection(e->dest_ip, e->dest_port);
    if (idx >= 0) {
        connections[idx].bytes_sent += e->bytes;
    }

    bytes_sent_total += e->bytes;

    char dest_ip_str[16];
    char dest_port_str[8];
    ip_to_string(e->dest_ip, dest_ip_str, sizeof(dest_ip_str));
    __builtin_sprintf(dest_port_str, "%d", e->dest_port);

    emit_counter_with_labels(
        "bytes_sent_total",
        e->bytes,
        "dest_ip", dest_ip_str,
        "dest_port", dest_port_str,
        NULL, NULL
    );
}

static void process_recv(struct network_event* e) {
    int idx = find_connection(e->dest_ip, e->dest_port);
    if (idx >= 0) {
        connections[idx].bytes_received += e->bytes;
    }

    bytes_received_total += e->bytes;

    char src_ip_str[16];
    char src_port_str[8];
    ip_to_string(e->dest_ip, src_ip_str, sizeof(src_ip_str));
    __builtin_sprintf(src_port_str, "%d", e->src_port);

    emit_counter_with_labels(
        "bytes_received_total",
        e->bytes,
        "src_ip", src_ip_str,
        "src_port", src_port_str,
        NULL, NULL
    );
}

// ============================================================================
// Exported Functions
// ============================================================================

__attribute__((export))
int epbf_init(void) {
    // Initialize connections array
    for (int i = 0; i < MAX_CONNECTIONS; i++) {
        connections[i].active = false;
        connections[i].dest_ip = 0;
        connections[i].dest_port = 0;
    }

    // Subscribe to eBPF map
    epbf_subscribe_map("network_events", 14);

    log_info("Network monitor initialized");
    return 0;
}

__attribute__((export))
void process_events(void) {
    // In a real implementation, this would read from the eBPF ring buffer
    // Events would be processed as they arrive
}

__attribute__((export))
void epbf_cleanup(void) {
    log_info("Network monitor cleanup");
}

// ============================================================================
// Statistics Functions
// ============================================================================

__attribute__((export))
uint64_t get_total_connections(void) {
    return tcp_connections_total;
}

__attribute__((export))
uint64_t get_total_bytes_sent(void) {
    return bytes_sent_total;
}

__attribute__((export))
uint64_t get_total_bytes_received(void) {
    return bytes_received_total;
}

__attribute__((export))
int get_active_connection_count(void) {
    int count = 0;
    for (int i = 0; i < MAX_CONNECTIONS; i++) {
        if (connections[i].active) count++;
    }
    return count;
}
