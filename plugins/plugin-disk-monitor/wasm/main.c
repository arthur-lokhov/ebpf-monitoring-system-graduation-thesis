// WASM plugin for disk I/O monitoring
// Processes disk events and emits Prometheus metrics

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

#define EVENT_TYPE_READ  1
#define EVENT_TYPE_WRITE 2

#define MAX_DEVICES 32
#define HISTOGRAM_BUCKETS 5

struct disk_event {
    uint64_t timestamp;
    uint32_t pid;
    uint32_t dev;
    uint64_t bytes;
    uint64_t latency_ns;
    char type;
    char comm[16];
    char filename[256];
};

struct device_stats {
    uint32_t dev;
    char name[32];
    uint64_t read_bytes;
    uint64_t write_bytes;
    uint64_t read_ops;
    uint64_t write_ops;
    uint64_t io_time_ns;
    uint64_t queue_length;
    bool active;
};

// Histogram buckets for I/O size
static const uint64_t io_size_buckets[HISTOGRAM_BUCKETS] = {512, 4096, 65536, 1048576, 10485760};

// ============================================================================
// Global State
// ============================================================================

static struct device_stats devices[MAX_DEVICES];
static uint64_t io_size_histogram[HISTOGRAM_BUCKETS + 1];  // +1 for +Inf
static uint64_t total_read_ops = 0;
static uint64_t total_write_ops = 0;

// ============================================================================
// Helper Functions
// ============================================================================

static void log_info(const char* msg) {
    epbf_log(LOG_INFO, msg, 0);
}

static int find_device(uint32_t dev) {
    for (int i = 0; i < MAX_DEVICES; i++) {
        if (devices[i].dev == dev && devices[i].active) {
            return i;
        }
    }
    return -1;
}

static int find_or_create_device(uint32_t dev) {
    int idx = find_device(dev);
    if (idx >= 0) {
        return idx;
    }

    for (int i = 0; i < MAX_DEVICES; i++) {
        if (!devices[i].active) {
            devices[i].dev = dev;
            devices[i].read_bytes = 0;
            devices[i].write_bytes = 0;
            devices[i].read_ops = 0;
            devices[i].write_ops = 0;
            devices[i].io_time_ns = 0;
            devices[i].queue_length = 0;
            devices[i].active = true;
            __builtin_sprintf(devices[i].name, "dev_%d", dev);
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

static int get_io_size_bucket(uint64_t bytes) {
    for (int i = 0; i < HISTOGRAM_BUCKETS; i++) {
        if (bytes <= io_size_buckets[i]) {
            return i;
        }
    }
    return HISTOGRAM_BUCKETS;  // +Inf bucket
}

// ============================================================================
// Event Processing
// ============================================================================

static void process_read(struct disk_event* e) {
    int idx = find_or_create_device(e->dev);
    if (idx < 0) {
        log_info("Device table full");
        return;
    }

    devices[idx].read_bytes += e->bytes;
    devices[idx].read_ops++;
    devices[idx].io_time_ns += e->latency_ns;
    total_read_ops++;

    // Update histogram
    int bucket = get_io_size_bucket(e->bytes);
    io_size_histogram[bucket]++;

    // Emit metrics
    emit_counter_with_labels(
        "disk_read_bytes_total",
        e->bytes,
        "device", devices[idx].name,
        "filename", e->filename[0] ? e->filename : "unknown",
        NULL, NULL
    );

    emit_counter_with_labels(
        "disk_read_ops_total",
        1,
        "device", devices[idx].name,
        NULL, NULL,
        NULL, NULL
    );

    // Emit histogram observation
    emit_histogram_with_labels(
        "disk_io_size_bytes",
        (double)e->bytes,
        "operation", "read"
    );
}

static void process_write(struct disk_event* e) {
    int idx = find_or_create_device(e->dev);
    if (idx < 0) {
        log_info("Device table full");
        return;
    }

    devices[idx].write_bytes += e->bytes;
    devices[idx].write_ops++;
    devices[idx].io_time_ns += e->latency_ns;
    total_write_ops++;

    // Update histogram
    int bucket = get_io_size_bucket(e->bytes);
    io_size_histogram[bucket]++;

    // Emit metrics
    emit_counter_with_labels(
        "disk_write_bytes_total",
        e->bytes,
        "device", devices[idx].name,
        "filename", e->filename[0] ? e->filename : "unknown",
        NULL, NULL
    );

    emit_counter_with_labels(
        "disk_write_ops_total",
        1,
        "device", devices[idx].name,
        NULL, NULL,
        NULL, NULL
    );

    // Emit histogram observation
    emit_histogram_with_labels(
        "disk_io_size_bytes",
        (double)e->bytes,
        "operation", "write"
    );
}

// ============================================================================
// Exported Functions
// ============================================================================

__attribute__((export))
int epbf_init(void) {
    // Initialize devices array
    for (int i = 0; i < MAX_DEVICES; i++) {
        devices[i].active = false;
        devices[i].dev = 0;
    }

    // Initialize histogram
    for (int i = 0; i <= HISTOGRAM_BUCKETS; i++) {
        io_size_histogram[i] = 0;
    }

    // Subscribe to eBPF map
    epbf_subscribe_map("disk_events", 11);

    log_info("Disk monitor initialized");
    return 0;
}

__attribute__((export))
void process_events(void) {
    // In a real implementation, this would read from the eBPF ring buffer
}

__attribute__((export))
void epbf_cleanup(void) {
    log_info("Disk monitor cleanup");
}

// ============================================================================
// Statistics Functions
// ============================================================================

__attribute__((export))
uint64_t get_total_read_bytes(void) {
    uint64_t total = 0;
    for (int i = 0; i < MAX_DEVICES; i++) {
        if (devices[i].active) {
            total += devices[i].read_bytes;
        }
    }
    return total;
}

__attribute__((export))
uint64_t get_total_write_bytes(void) {
    uint64_t total = 0;
    for (int i = 0; i < MAX_DEVICES; i++) {
        if (devices[i].active) {
            total += devices[i].write_bytes;
        }
    }
    return total;
}

__attribute__((export))
uint64_t get_total_read_ops(void) {
    return total_read_ops;
}

__attribute__((export))
uint64_t get_total_write_ops(void) {
    return total_write_ops;
}

__attribute__((export))
int get_device_count(void) {
    int count = 0;
    for (int i = 0; i < MAX_DEVICES; i++) {
        if (devices[i].active) count++;
    }
    return count;
}

__attribute__((export))
uint64_t get_histogram_bucket(int bucket) {
    if (bucket < 0 || bucket > HISTOGRAM_BUCKETS) {
        return 0;
    }
    return io_size_histogram[bucket];
}
