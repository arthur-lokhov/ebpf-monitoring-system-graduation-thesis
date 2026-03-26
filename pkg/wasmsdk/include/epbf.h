/*
 * epbf.h - WASM SDK for eBPF Monitoring Plugins
 * 
 * This header provides the API for writing WASM plugins that interact
 * with eBPF programs and emit metrics to the monitoring platform.
 * 
 * Usage:
 *   #include "epbf.h"
 * 
 *   int epbf_init(void) {
 *       // Initialization code
 *       return 0;
 *   }
 * 
 *   void process_events() {
 *       // Subscribe to eBPF map
 *       int map_fd = epbf_subscribe_map("tcp_events");
 *       
 *       // Read and process events
 *       // ...
 *       
 *       // Emit metrics
 *       label_t labels[] = {
 *           {"interface", "eth0"},
 *           {"protocol", "tcp"}
 *       };
 *       epbf_emit_counter("bytes_sent", bytes, labels, 2);
 *   }
 */

#ifndef EPBF_WASM_SDK_H
#define EPBF_WASM_SDK_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ============================================================================
 * Types
 * ============================================================================ */

/**
 * Metric types supported by the platform
 */
typedef enum {
    EPBF_METRIC_COUNTER,    /**< Monotonically increasing value */
    EPBF_METRIC_GAUGE,      /**< Value that can go up or down */
    EPBF_METRIC_HISTOGRAM,  /**< Distribution of values in buckets */
    EPBF_METRIC_SUMMARY     /**< Quantiles and summary statistics */
} epbf_metric_type_t;

/**
 * Log levels for plugin logging
 */
typedef enum {
    EPBF_LOG_DEBUG,
    EPBF_LOG_INFO,
    EPBF_LOG_WARN,
    EPBF_LOG_ERROR
} epbf_log_level_t;

/**
 * Label for metrics (key-value pair)
 */
typedef struct {
    const char* key;
    const char* value;
} epbf_label_t;

/**
 * Histogram bucket
 */
typedef struct {
    double le;        /**< Less than or equal boundary */
    uint64_t count;   /**< Count of observations in this bucket */
} epbf_histogram_bucket_t;

/* ============================================================================
 * Lifecycle
 * ============================================================================ */

/**
 * Initialize the plugin
 * 
 * This function is called once when the plugin is loaded.
 * Use it to perform any initialization tasks.
 * 
 * @return 0 on success, non-zero on error
 */
int epbf_init(void);

/**
 * Plugin cleanup function
 * 
 * Called when the plugin is being unloaded.
 */
void epbf_cleanup(void);

/* ============================================================================
 * eBPF Map Operations
 * ============================================================================ */

/**
 * Subscribe to an eBPF map
 * 
 * Creates a subscription to receive events from the specified eBPF map.
 * The map must be defined in the plugin's eBPF program.
 * 
 * @param map_name Name of the eBPF map (as defined in manifest.yml)
 * @return Map file descriptor on success, -1 on error
 */
int epbf_subscribe_map(const char* map_name);

/**
 * Read a value from an eBPF map
 * 
 * @param map_name Name of the eBPF map
 * @param key Pointer to the key
 * @param key_size Size of the key in bytes
 * @param value Pointer to buffer for the value
 * @param value_size Size of the value buffer
 * @return 0 on success, -1 on error
 */
int epbf_read_map(const char* map_name, 
                  const void* key, size_t key_size,
                  void* value, size_t value_size);

/**
 * Update a value in an eBPF map
 * 
 * @param map_name Name of the eBPF map
 * @param key Pointer to the key
 * @param key_size Size of the key in bytes
 * @param value Pointer to the value
 * @param value_size Size of the value
 * @return 0 on success, -1 on error
 */
int epbf_update_map(const char* map_name,
                    const void* key, size_t key_size,
                    const void* value, size_t value_size);

/**
 * Delete a value from an eBPF map
 * 
 * @param map_name Name of the eBPF map
 * @param key Pointer to the key
 * @param key_size Size of the key in bytes
 * @return 0 on success, -1 on error
 */
int epbf_delete_map_key(const char* map_name, 
                        const void* key, size_t key_size);

/* ============================================================================
 * Metric Emission
 * ============================================================================ */

/**
 * Emit a counter metric
 * 
 * @param name Metric name (must be defined in manifest.yml)
 * @param value Counter value (must be monotonically increasing)
 * @param labels Array of labels
 * @param label_count Number of labels
 */
void epbf_emit_counter(const char* name, uint64_t value,
                       epbf_label_t* labels, size_t label_count);

/**
 * Emit a gauge metric
 * 
 * @param name Metric name (must be defined in manifest.yml)
 * @param value Gauge value (can go up or down)
 * @param labels Array of labels
 * @param label_count Number of labels
 */
void epbf_emit_gauge(const char* name, double value,
                     epbf_label_t* labels, size_t label_count);

/**
 * Emit a histogram metric
 * 
 * @param name Metric name (must be defined in manifest.yml)
 * @param buckets Array of histogram buckets
 * @param bucket_count Number of buckets
 * @param sum Sum of all observed values
 * @param count Total count of observations
 * @param labels Array of labels
 * @param label_count Number of labels
 */
void epbf_emit_histogram(const char* name,
                         epbf_histogram_bucket_t* buckets, size_t bucket_count,
                         double sum, uint64_t count,
                         epbf_label_t* labels, size_t label_count);

/**
 * Observe a single value (for histogram/summary)
 * 
 * Convenience function to observe a single value.
 * The platform handles bucketing automatically.
 * 
 * @param name Metric name (must be histogram or summary type)
 * @param value Observed value
 * @param labels Array of labels
 * @param label_count Number of labels
 */
void epbf_observe(const char* name, double value,
                  epbf_label_t* labels, size_t label_count);

/* ============================================================================
 * Logging
 * ============================================================================ */

/**
 * Log a message
 * 
 * @param level Log level
 * @param format Printf-style format string
 * @param ... Format arguments
 */
void epbf_log(epbf_log_level_t level, const char* format, ...);

/**
 * Log a debug message
 */
#define EPBF_LOG_DEBUG(fmt, ...) \
    epbf_log(EPBF_LOG_DEBUG, fmt, ##__VA_ARGS__)

/**
 * Log an info message
 */
#define EPBF_LOG_INFO(fmt, ...) \
    epbf_log(EPBF_LOG_INFO, fmt, ##__VA_ARGS__)

/**
 * Log a warning message
 */
#define EPBF_LOG_WARN(fmt, ...) \
    epbf_log(EPBF_LOG_WARN, fmt, ##__VA_ARGS__)

/**
 * Log an error message
 */
#define EPBF_LOG_ERROR(fmt, ...) \
    epbf_log(EPBF_LOG_ERROR, fmt, ##__VA_ARGS__)

/* ============================================================================
 * Time
 * ============================================================================ */

/**
 * Get current time in nanoseconds since epoch
 * 
 * @return Current time in nanoseconds
 */
uint64_t epbf_now_ns(void);

/**
 * Get current time in milliseconds since epoch
 * 
 * @return Current time in milliseconds
 */
uint64_t epbf_now_ms(void);

/**
 * Get current time in seconds since epoch
 * 
 * @return Current time in seconds
 */
uint64_t epbf_now_s(void);

/* ============================================================================
 * Timers
 * ============================================================================ */

/**
 * Set up a periodic timer
 * 
 * @param interval_ms Interval in milliseconds
 * @return Timer ID on success, -1 on error
 */
int epbf_set_interval(uint64_t interval_ms);

/**
 * Clear a timer
 * 
 * @param timer_id Timer ID to clear
 */
void epbf_clear_timer(int timer_id);

/**
 * Sleep for a duration
 * 
 * @param duration_ms Duration in milliseconds
 */
void epbf_sleep(uint64_t duration_ms);

/* ============================================================================
 * Plugin Info
 * ============================================================================ */

/**
 * Get the plugin name
 * 
 * @return Plugin name as defined in manifest.yml
 */
const char* epbf_get_plugin_name(void);

/**
 * Get the plugin version
 * 
 * @return Plugin version as defined in manifest.yml
 */
const char* epbf_get_plugin_version(void);

#ifdef __cplusplus
}
#endif

#endif /* EPBF_WASM_SDK_H */
