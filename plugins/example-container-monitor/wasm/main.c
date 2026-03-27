// Minimal WASM plugin for container monitoring
// This is a stub that will be expanded

#include <stdint.h>

// Exported functions (called by runtime)
__attribute__((export))
int epbf_init(void) {
    return 0;
}

__attribute__((export))
void process_events(void) {
    // Event processing stub
}

__attribute__((export))
void epbf_cleanup(void) {
    // Cleanup stub
}
